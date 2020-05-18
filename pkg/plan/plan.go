package plan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/fatih/structs"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/utilities/object"
)

// Runner is something that can realise a step.
type Runner interface {
	// RunCommand runs a command in a shell. This means cmd can be more than one
	// single command, it can be a full bourne shell script.
	RunCommand(cmd string, stdin io.Reader) (stdouterr string, err error)
}

// Resource is an atomic step of the plan.
type Resource interface {
	// State returns the state that this step will realize when applied.
	State() State
	// QueryState returns the current state of this step. For instance, if the step
	// describes the installation of a package, QueryState will return if the
	// package is actually installed and its version.
	QueryState(runner Runner) (State, error)

	// Apply this step and indicate whether downstream resources should be re-applied
	Apply(runner Runner, diff Diff) (propagate bool, err error)
	// Undo this step.
	Undo(runner Runner, current State) error
}

type RunError struct {
	ExitCode int
}

func (e *RunError) Error() string {
	return fmt.Sprintf("command exited with %d", e.ExitCode)
}

// Plan is a succession of Steps to produce a desired outcome.
type Plan struct {
	id            string
	resources     map[string]Resource
	graph         *graph
	undoCondition func(Runner, State) bool
}

var (
	dummyPlan    Resource = RegisterResource(&Plan{})
	planTypeName          = extractResourceTypeName(dummyPlan)
)

// ParamString is a parameterizable string for passing output from one resource
// to another. The model is:
//
// A "Run" command (and possibly others?) can take an extra "Output" parameter which is the
// address of a string variable. The variable will get populated with the output of the command.
//
// A downstream resource can pass a "ParamString" along with any associated string variable addresses
// and the parameters will get filled in at runtime (after the upstream resource has run).
// "plan.ParamString(template, params...)" will create a ParamString whose "String()" method will return
// an instantiated string. If no parameters are necessary, "object.String(str)" can be used to make the intent
// more clear.
//
// Examples:
//
//	var k8sVersion string
//
//  b.AddResource(
//		"install:cni:get-k8s-version",
//		&resource.Run{
//			Script: object.String("kubectl version | base64 | tr -d '\n'"),
//			Output: &k8sVersion,
//		},
//		plan.DependOn("kubeadm:init"),
//  ).AddResource(
//		"install:cni",
//		&resource.KubectlApply{
//			ManifestURL: plan.ParamString("https://cloud.weave.works/k8s/net?k8s-version=%s", &k8sVersion),
//		},
//		plan.DependOn("install:cni:get-k8s-version"),
//  )
//
//  var homedir string
//
//  b.AddResource(
//		"kubeadm:get-homedir",
//		&Run{Script: object.String("echo -n $HOME"), Output: &homedir},
//  ).AddResource(
//		"kubeadm:config:kubectl-dir",
//		&Dir{Path: plan.ParamString("%s/.kube", &homedir)},
//		plan.DependOn("kubeadm:get-homedir"),
//  ).AddResource(
//		"kubeadm:config:copy",
//		&Run{Script: plan.ParamString("cp /etc/kubernetes/admin.conf %s/.kube/config", &homedir)},
//		plan.DependOn("kubeadm:run-init", "kubeadm:config:kubectl-dir"),
//  ).AddResource(
//		"kubeadm:config:set-ownership",
//		&Run{Script: plan.ParamString("chown $(id -u):$(id -g) %s/.kube/config", &homedir)},
//		plan.DependOn("kubeadm:config:copy"),
//  )

type paramTemplateString struct {
	template string
	params   []*string
}

func (p *paramTemplateString) String() string {
	strs := make([]interface{}, len(p.params))
	for i := range p.params {
		strs[i] = *p.params[i]
	}
	return fmt.Sprintf(p.template, strs...)
}

func ParamString(template string, params ...*string) fmt.Stringer {
	if len(params) == 0 {
		return object.String(template)
	}
	return &paramTemplateString{template, params}
}

// Diff represents the motivation for performing an Apply; a
// cached version of the current State and any dependee Resources that have
// been invalidated.
type Diff struct {
	CurrentState    State
	InvalidatedDeps []Resource
}

// Validity is a Resource's measured "goodness"
// Either 'Valid', 'Invalid', or 'Inconclusive'.
// The state is 'Inconclusive' if a state query
//   fails in the validity tree
type Validity int

const (
	Valid Validity = iota
	Invalid
	Inconclusive
)

// InvalidityReason describes why a resource isn't valid
type InvalidityReason int

const (
	None InvalidityReason = iota
	ApplyError
	QueryError
	ChildInvalid
	ChildInconclusive
	DependencyInvalid
	DependencyInconclusive
)

// internal struct used to connect resource graph nodes via channels
type connectors struct {
	inCount int
	in      chan ValidityTree
	outs    []chan ValidityTree
}

// ValidityTree is the hierarchical explanation for why a resource
// is Valid, Invalid, or Inconclusive. Each ValidityTree contains
// a set of dependency ValidityTrees showing how the Validity status
// rolls up
type ValidityTree struct {
	ResourceID     string           `json:"resource"`
	ValidityStatus Validity         `json:"status,omitempty"`
	Reason         InvalidityReason `json:"reason,omitempty"`
	Updated        bool             `json:"updated,omitempty"`
	ObservedError  string           `json:"error,omitempty"`
	Dependencies   []ValidityTree   `json:"dependencies,omitempty"`
	Children       []ValidityTree   `json:"children,omitempty"`
}

// EmptyDiff returns a default Diff value
func EmptyDiff() Diff {
	return Diff{
		CurrentState:    nil,
		InvalidatedDeps: []Resource{}}
}

// Functions to translate enums to/from strings for JSON
func (v Validity) String() string {
	return toValidityString[v]
}

var toValidityString = map[Validity]string{
	Valid:        "Valid",
	Invalid:      "Invalid",
	Inconclusive: "Inconclusive",
}

var toValidityID = map[string]Validity{
	"Valid":        Valid,
	"Invalid":      Invalid,
	"Inconclusive": Inconclusive,
}

// MarshalJSON marshals the enum as a quoted json string
func (v Validity) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(toValidityString[v])
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

// UnmarshalJSON unmashals a quoted json string to the enum value
func (v *Validity) UnmarshalJSON(b []byte) error {
	var j string
	err := json.Unmarshal(b, &j)
	if err != nil {
		return err
	}
	// Note that if the string cannot be found then it will be set to the zero value,
	// 'Valid' in this case.
	*v = toValidityID[j]
	return nil
}

func (r InvalidityReason) String() string {
	return toReasonString[r]
}

var toReasonString = map[InvalidityReason]string{
	None:                   "None",
	ApplyError:             "ApplyError",
	QueryError:             "QueryError",
	ChildInvalid:           "ChildInvalid",
	ChildInconclusive:      "ChildInconclusive",
	DependencyInvalid:      "DependencyInvalid",
	DependencyInconclusive: "DependencyInconclusive",
}

var toReasonID = map[string]InvalidityReason{
	"None":                   None,
	"ApplyError":             ApplyError,
	"QueryError":             QueryError,
	"ChildInvalid":           ChildInvalid,
	"ChildInconclusive":      ChildInconclusive,
	"DependencyInvalid":      DependencyInvalid,
	"DependencyInconclusive": DependencyInconclusive,
}

// MarshalJSON marshals the enum as a quoted json string
func (r InvalidityReason) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(toReasonString[r])
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

// UnmarshalJSON unmashals a quoted json string to the enum value
func (r *InvalidityReason) UnmarshalJSON(b []byte) error {
	var j string
	err := json.Unmarshal(b, &j)
	if err != nil {
		return err
	}
	// Note that if the string cannot be found then it will be set to the zero value,
	// 'None' in this case.
	*r = toReasonID[j]
	return nil
}

// ToJSON returns a JSON representation of the ValidityTree.
func (v ValidityTree) ToJSON() string {
	str, _ := json.MarshalIndent(v, "", " ")
	return string(str)
}

func (v ValidityTree) prune(seen map[string]bool) ValidityTree {
	if seen[v.ResourceID] {
		return ValidityTree{ResourceID: v.ResourceID}
	}
	seen[v.ResourceID] = true
	copy := v
	copy.Children = []ValidityTree{}
	copy.Dependencies = []ValidityTree{}
	for _, child := range v.Children {
		copy.Children = append(copy.Children, child.prune(seen))
	}
	for _, dep := range v.Dependencies {
		copy.Dependencies = append(copy.Dependencies, dep.prune(seen))
	}
	return copy
}

// ToExplanation removes all VALID resource dependencies before
// producing JSON to make it more clear what went wrong.
// It also only shows full data for a tree the first time it's resource is seen.
func (v ValidityTree) ToExplanation() ValidityTree {
	copy := v
	newChildren := []ValidityTree{}
	newDependencies := []ValidityTree{}
	for _, child := range copy.Children {
		if child.ValidityStatus != Valid {
			newChildren = append(newChildren, child.ToExplanation())
		}
	}
	for _, dep := range copy.Dependencies {
		if dep.ValidityStatus != Valid {
			newDependencies = append(newDependencies, dep.ToExplanation())
		}
	}
	copy.Children = newChildren
	copy.Dependencies = newDependencies
	return copy.prune(make(map[string]bool))
}

func (v ValidityTree) ObservedErrorString() string {
	if v.ObservedError == "" {
		return "an unspecified error"
	}
	return v.ObservedError
}

var errorStrings = map[InvalidityReason]string{
	QueryError:             "the current state could not be determined",
	ChildInvalid:           "a child failed",
	ChildInconclusive:      "the current state of a child could not be determined",
	DependencyInvalid:      "a dependency failed",
	DependencyInconclusive: "the current state of a dependency could not be determined",
}

func (v ValidityTree) Error() string {
	prefix := "Apply failed because "
	if v.Reason == ApplyError {
		return prefix + "of: " + v.ObservedErrorString()
	}
	return prefix + errorStrings[v.Reason]
}

func newPlan() *Plan {
	return &Plan{
		resources: make(map[string]Resource),
		graph:     newGraph(16),
	}
}

func (p *Plan) addResource(id string, r Resource, options *addOptions) error {
	assertRegistered(r)
	if _, ok := p.resources[id]; ok {
		return fmt.Errorf("plan: resource %q already in the plan", id)
	}
	if plan, ok := r.(*Plan); ok {
		plan.id = id
	}
	p.resources[id] = r
	p.graph.AddNode(id)
	for _, dep := range options.deps {
		p.graph.AddEdge(dep, id)
	}
	return nil
}

func (p *Plan) GetResource(name string) Resource {
	return p.resources[name]
}

// Support for serializing Plans via transformation to States

// We save the base struct type and whether the actual Resource type
// was a pointer so we can tell whether or not to take a struct's
// address after creating it during deserialization
type resourceType struct {
	baseType reflect.Type
	isPtr    bool
}

// ToJSON translates a Plan into JSON using State as an intermediate target
func (p *Plan) ToJSON() string {
	return displayableState(p.toState()).Marshal()
}

// Skip empty fields marked as "omitempty" in DOT as well as JSON
func isEmptyField(elem reflect.Value, idx int) bool {
	t := elem.Type()
	f := t.Field(idx)
	if f.PkgPath != "" {
		// unexported
		return true
	}
	z := reflect.Zero(f.Type)
	val := elem.Field(idx)
	if reflect.DeepEqual(val.Interface(), z.Interface()) {
		tag, ok := f.Tag.Lookup("structs")
		if ok {
			tagfields := strings.Split(tag, ",")
			for _, s := range tagfields {
				if s == "omitempty" {
					return true
				}
			}
		}
	}
	return false
}

// ToDOT translates a Plan into DOT output
func (p *Plan) ToDOT() string {
	var dot bytes.Buffer
	dot.WriteString("digraph wks {\n")
	dot.WriteString("\t node [shape=box, style=bold]\n")
	dot.WriteString("\t ranksep=\"1.0 equally\"\n")
	p.toDOTBody(&dot)
	p.childrenToDOT(&dot)
	dot.WriteString("}")
	return dot.String()
}

func (p *Plan) childrenToDOT(dot *bytes.Buffer) {
	children, _ := p.graph.Toposort()
	for _, child := range children {
		dot.WriteString(fmt.Sprintf("\t\"%s\" -> \"%s\" [style=dashed color=red]\n", child, p.id))
	}
}

func (p *Plan) toDOTBody(dot *bytes.Buffer) {
	resDeps := extractDependencies(p.graph)
	for name, res := range p.resources {
		pres, isPlan := res.(*Plan)
		dot.WriteString(fmt.Sprintf("\t\"%s\"", name))
		k := reflect.ValueOf(res).Kind()
		if k == reflect.Interface || k == reflect.Ptr {
			resElem := reflect.ValueOf(res).Elem()
			dot.WriteString(fmt.Sprintf(" [label=<<TABLE BORDER=\"0\"><TR><TD COLSPAN=\"2\" align=\"center\">%s<BR/>%s</TD></TR>",
				name, resElem.Type()))
			style := "rounded"
			if isPlan {
				style = "bold"
			}
			dot.WriteString(fmt.Sprintf("</TABLE>> style=%s]\n", style))
			if isPlan {
				pres.toDOTBody(dot)
			}
		}
		deps := resDeps[name]
		for _, d := range deps {
			dot.WriteString(fmt.Sprintf("\t\"%s\" -> \"%s\" [style=bold color=blue]\n", d, name))
		}
		if isPlan {
			pres.childrenToDOT(dot)
		}
		dot.WriteString("\n")
	}
}

// NewPlanFromJSON Reads a Plan from JSON via State
func NewPlanFromJSON(r io.Reader) (Plan, error) {
	s, err := NewStateFromJSON(r)
	if err != nil {
		return *newPlan(), err
	}
	return s.toPlan(), nil
}

// The approach taken here is to minimize the requirements placed on
// Resource implementors. At the bare minimum, it's necessary to
// register some mapping from a resource type name to a means for
// constructing one of its instance on deserialization. We are, in
// fact, requiring only the minimum: Each Resource type needs only to
// invoke RegisterResource() once on an instance of its type which
// creates the mapping. Conveniently, we already create a dummy
// instance of each Resource type to ensure that it implements the
// Resource interface. All that is needed is to wrap that instance in
// a RegisterResource() call.  If we don't perform registration at
// initialization time, it's possible we could attempt to read in a
// Plan before one or more of its Resources had been seen.

// If an attempt is made to add an unregistered Resource to a Plan, a
// fatal error is generated.

// To reduce noise in the JSON output, we don't save the package of a
// resource unless it lives outside the default 'resource'
// package. This var stores that package path so we can compare
// against it when serializing.
var resourcePackageName = filepath.Join(reflect.ValueOf(EmptyState).Type().PkgPath(), "resource")

// A map of Resource names (some package qualified as above) to types. The types are used to construct
// instances via reflection on load.
var resourceTypes = make(map[string]resourceType)

// RegisterResource is used to map a Resource type name to its type information for deserialization
func RegisterResource(r Resource) Resource {
	resourceTypes[extractResourceTypeName(r)] = resourceBaseType(r)
	return r
}

// Function that will error out if an unregistered Resource type is used
func assertRegistered(r Resource) {
	if _, ok := resourceTypes[extractResourceTypeName(r)]; !ok {
		log.Fatalf("Resource type: '%v' not registered. "+
			"Please insert a line such as the following in your resource's source file: \""+
			"var _ plan.Resource = plan.RegisterResource(<resource instance>)\"", reflect.TypeOf(r))
	}
}

func resourceTypeByName(name string) resourceType {
	return resourceTypes[name]
}

// EqualPlans Compares Plans for equality; not currently used except in tests
func EqualPlans(p1, p2 Plan) bool {
	return p1.ToJSON() == p2.ToJSON()
}

func graphsEqual(g1, g2 *graph) bool {
	entrymap := make(map[interface{}]bool)
	for node := range g1.nodes {
		entrymap[node] = true
	}
	for node := range g2.nodes {
		if !entrymap[node] {
			return false
		}
		delete(entrymap, node)
	}
	if len(entrymap) > 0 {
		return false
	}
	for from, tos := range g1.edges {
		for to := range tos {
			entrymap[edge{from, to}] = true
		}
	}
	for from, tos := range g2.edges {
		for to := range tos {
			e := edge{from, to}
			if !entrymap[e] {
				return false
			}
			delete(entrymap, e)
		}
	}
	return len(entrymap) == 0
}

// Main worker function for serialization
func (p *Plan) toState() State {
	planData := make(map[string]interface{})
	deps := extractDependencies(p.graph)
	for id, r := range p.resources {
		// typename is package-qualified if outside of the 'resource' package
		typename := extractResourceTypeName(r)
		depends := deps[id]
		if depends == nil {
			depends = []string{}
		}
		meta := map[string]interface{}{"dependsOn": depends}
		resData := structs.Map(r)
		if planResource, ok := r.(*Plan); ok {
			resData = planResource.toState()
		}
		planData[id] = map[string]interface{}{
			"meta":   meta,
			typename: resData,
		}
	}
	return State(planData)
}

func displayableState(planState State) State {
	m := map[string]interface{}{}
	for id, resval := range planState {
		// Plan state is a map of string ids to State objects
		resmap := resval.(map[string]interface{})
		m[id] = displayableResourceState(resmap)
	}
	return State(m)
}

func displayableResourceState(stateEntry map[string]interface{}) map[string]interface{} {
	// A resource map has a "meta" entry containing dependencies and an entry named by
	// its type
	resourceMap := map[string]interface{}{}
	resourceMap["meta"] = stateEntry["meta"]
	for typeName, valstate := range stateEntry {
		if typeName != "meta" {
			resourceMap[typeName] = displayableResourceData(typeName, valstate)
			break
		}
	}
	return resourceMap
}

func displayableResourceData(typeName string, stateData interface{}) State {
	// Plans are just translated recursively
	if typeName == planTypeName {
		return displayableState(State(stateData.(map[string]interface{})))
	} else {
		// populate a map by scanning through the resource fields and skipping
		// hidden ones
		rtype := resourceTypeByName(typeName)
		rmap := map[string]interface{}{}
		for i := 0; i < rtype.baseType.NumField(); i++ {
			// ignore fields marked by the `plan:"hide"` tag
			f := rtype.baseType.Field(i)
			planTag := f.Tag.Get("plan")
			if planTag == "hide" {
				continue
			}
			key := f.Name
			structsTag := f.Tag.Get("structs")
			if structsTag != "" {
				// find the actual name to use as the map key in the structs tag, if specified
				tagItems := strings.Split(structsTag, ",")
				key = tagItems[0]
			}
			// skip missing map entries since they were remove by "omitempty" struct tags
			val, ok := stateData.(map[string]interface{})[key]
			if ok {
				rmap[key] = val
			}
		}
		return State(rmap)
	}
}

// Return the struct type defining the Resource and whether or not the
// Resource itself is defined as a pointer
func resourceBaseType(r Resource) resourceType {
	val := reflect.ValueOf(r)
	isPtr := (val.Kind() == reflect.Ptr)
	if isPtr {
		val = reflect.Indirect(val)
	}
	return resourceType{val.Type(), isPtr}
}

// Return the name we will use to identify the Resource type in JSON
func extractResourceTypeName(r Resource) string {
	var typename string
	btype := resourceBaseType(r).baseType
	ppath := btype.PkgPath()
	if ppath != resourcePackageName {
		typename = filepath.Join(ppath, btype.Name())
	} else {
		typename = btype.Name()
	}
	return typename
}

// Collect (Resource --> Dependencies) for all Resources in the Plan
func extractDependencies(graphval *graph) map[string][]string {
	dependsOn := make(map[string][]string)
	for from, tos := range graphval.edges {
		for to := range tos {
			dependencies := dependsOn[to]
			if dependencies == nil {
				dependencies = make([]string, 0)
			}
			dependencies = append(dependencies, from)
			sort.Strings(dependencies)
			dependsOn[to] = dependencies
		}
	}
	return dependsOn
}

// Sort dependencies by their resource ids
func sortTreeDeps(deps []ValidityTree) {
	sort.Slice(deps,
		func(i, j int) bool {
			return strings.Compare(deps[i].ResourceID, deps[j].ResourceID) < 0
		})
}

// Check for a status value across a set of dependency ValidityTrees
func any(depTrees map[string]ValidityTree, pred func(ValidityTree) bool) bool {
	for _, vtree := range depTrees {
		if pred(vtree) {
			return true
		}
	}
	return false
}

// Create a ValidityTree for the plan resource currently being applied
func buildErrorResultTree(
	id string,
	vmap map[string]ValidityTree,
	status Validity,
	reason InvalidityReason) ValidityTree {
	childValidities := make([]ValidityTree, 0)
	for _, vtree := range vmap {
		childValidities = append(childValidities, vtree)
	}
	sortTreeDeps(childValidities)
	return ValidityTree{
		ResourceID:     id,
		ValidityStatus: status,
		Reason:         reason,
		ObservedError:  "Apply failed because " + errorStrings[reason],
		Children:       childValidities,
		Dependencies:   make([]ValidityTree, 0)}
}

func (p *Plan) endpoints() []string {
	endpoints := make([]string, 0)
	dependents := extractDependencies(p.graph.Invert())
	for rid := range p.resources {
		if len(dependents[rid]) == 0 {
			endpoints = append(endpoints, rid)
		}
	}
	return endpoints
}

// Apply implements Resource.

// Apply applies this plan.
// The diff contains:
//   1) either a state roll-up of contained resource states or nil
//   2) a set of updated resources (of which only direct dependencies will be handed to
//      individual resources
func (p *Plan) Apply(r Runner, diff Diff) (bool, error) {
	validity := p.ApplyResourceGraph(p.endpoints(), &diff, r)
	statusCheck := func(status Validity) bool {
		return any(validity, func(vtree ValidityTree) bool { return vtree.ValidityStatus == status })
	}
	if statusCheck(Invalid) {
		return false, buildErrorResultTree(p.id, validity, Invalid, ChildInvalid)
	}
	if statusCheck(Inconclusive) {
		return false, buildErrorResultTree(p.id, validity, Inconclusive, ChildInconclusive)
	}
	return any(validity, func(vtree ValidityTree) bool { return vtree.Updated }), nil
}

// State implements Resource
func (p *Plan) State() State {
	state := State(make(map[string]interface{}))
	for rid, r := range p.resources {
		state[rid] = r.State()
	}
	return state
}

// QueryState implements Resource
func (p *Plan) QueryState(runner Runner) (State, error) {
	state := State(make(map[string]interface{}))
	errors := make(map[string]error)
	for rid, r := range p.resources {
		s, err := r.QueryState(runner)
		if err != nil {
			errors[rid] = err
		} else {
			state[rid] = s
		}
	}
	if len(errors) != 0 {
		// Not necessarily a problem if this is the first time the plan has been applied; resources
		// may depend on system mods from earlier resources they depend on in order to query state.
		return EmptyState, nil
	}
	return state, nil
}

// Undo implements Resource
func (p *Plan) Undo(runner Runner, current State) error {
	if p.undoCondition != nil && !p.undoCondition(runner, current) {
		return nil
	}
	errors := make(map[string]error)
	sorted, _ := p.graph.Invert().Toposort() // undo in reverse order
	for _, rid := range sorted {
		logger := log.WithField("resource", rid)
		logger.Debug("Undoing")
		if err := p.resources[rid].Undo(runner, current); err != nil {
			errors[rid] = err
			logger.Error("Undo failed")
		} else {
			logger.Debug("Undone")
		}
	}
	if len(errors) != 0 {
		return fmt.Errorf("Partial undo completed due to the following errors:\n%s", formatNestedErrors(errors))
	}
	return nil
}

// SetUndoCondition sets the function that determines if the Plan will allow undoing its actions
func (p *Plan) SetUndoCondition(checkFun func(Runner, State) bool) {
	p.undoCondition = checkFun
}

func formatNestedErrors(errors map[string]error) string {
	var sb strings.Builder
	for id, err := range errors {
		sb.WriteString(id)
		sb.WriteString(" ")
		sb.WriteString(strings.Replace(err.Error(), "\n", "\n  ", -1))
		sb.WriteString("\n")
	}
	return sb.String()
}

func addPrerequisites(newg *graph, deps map[string][]string, resID string) {
	resDeps := deps[resID]
	for _, dep := range resDeps {
		newg.AddNode(dep)
		newg.AddEdge(dep, resID)
		addPrerequisites(newg, deps, dep)
	}
}

// Function to reduce a graph to only a specific node and the nodes underpinning it
func selectPrerequisites(ids []string, g *graph) *graph {
	deps := extractDependencies(g)
	newg := newGraph(len(g.nodes))
	for _, id := range ids {
		newg.AddNode(id)
		addPrerequisites(newg, deps, id)
	}
	return newg
}

func (p *Plan) ApplyResourceGraph(resourceIDs []string, diff *Diff, runner Runner) map[string]ValidityTree {
	underlyingGraph := selectPrerequisites(resourceIDs, p.graph)
	validity := p.applyResources(underlyingGraph, resourceIDs, diff, runner)
	for resourceID, resourceValidity := range validity {
		resourceValidityStatus := resourceValidity.ValidityStatus
		if resourceValidityStatus != Valid {
			log.Infof("State of Resource '%s' is %s.\nExplanation:\n%s\n",
				resourceID, toValidityString[resourceValidityStatus],
				resourceValidity.ToExplanation().ToJSON())
		}
	}
	return validity
}

// EnsureResourcesValid ensures that a set of resources is valid by recursively
// checking dependencies for validity and re-applying as necessary to
// achieve it; returns only results for the top-level resources as results for underlying
// resources will roll up
func (p *Plan) EnsureResourcesValid(resourceIDs []string, runner Runner) map[string]ValidityTree {
	return p.ApplyResourceGraph(resourceIDs, nil, runner)
}

// EnsureResourceValid ensures that a resource is valid by recursively
// checking dependencies for validity and re-applying as necessary to
// achieve it
func (p *Plan) EnsureResourceValid(resourceID string, runner Runner) ValidityTree {
	return p.EnsureResourcesValid([]string{resourceID}, runner)[resourceID]
}

func (p *Plan) applyResources(g *graph, endpoints []string, diff *Diff, runner Runner) map[string]ValidityTree {
	// Get a reverse graph so we can use extractDependencies to get dependents
	downwardGraph := g.Invert()
	// Get all the dependencies for each resource
	dependencies := extractDependencies(g)
	// Get all the dependents for each resource
	dependents := extractDependencies(downwardGraph)
	// Start from the bottom and work up
	sorted, _ := g.Toposort()
	// Create a top node to catch updates from endpoints
	top := &connectors{0, make(chan ValidityTree, len(sorted)), nil}
	// Connect all nodes in the graph with their dependents via channels
	connectors := p.createChannels(sorted, dependencies, dependents, endpoints, top)
	result := make(map[string]ValidityTree)
	// ascend the graph propagating validity
	if err := p.propagate(sorted, connectors, diff, runner); err != nil {
		return result
	}
	// Wait for all results to reach the top
	for i := 0; i < top.inCount; i++ {
		vt := <-top.in
		result[vt.ResourceID] = vt
	}
	return result
}

// For each resource, create an input channel if it has at least one
// dependency and connect it to the input channel for each of its
// dependents. If it has no dependents or was explicitly selected as an endpoint,
// attach the channel for the top-level sink which will be used to wait for all
// validations to complete.
func (p *Plan) createChannels(
	sortedResources []string,
	dependencies map[string][]string,
	dependents map[string][]string,
	explicitEndpoints []string,
	top *connectors) map[string]*connectors {
	resourceConnectors := make(map[string]*connectors)
	endpointMap := make(map[string]bool)
	for _, endpoint := range explicitEndpoints {
		endpointMap[endpoint] = true
	}
	// Each resource has one input channel and a set of output
	// channels.
	//
	//When a resource feeding in to a resource is connected,
	// the input count of the downstream resource is incremented so it
	// can determine when all dependees have finished
	getConnectors := func(id string) *connectors {
		conns, ok := resourceConnectors[id]
		if !ok {
			conns = &connectors{0, make(chan ValidityTree, len(dependencies[id])), make([]chan ValidityTree, 0)}
			resourceConnectors[id] = conns
		}
		return conns
	}
	// Connect each dependent input to this resource's output
	for _, res := range sortedResources {
		resConns := getConnectors(res)
		deps := dependents[res]
		if len(deps) == 0 || endpointMap[res] { // an endpoint
			resConns.outs = append(resConns.outs, top.in)
			top.inCount++
		}
		if len(deps) > 0 {
			for _, dep := range deps {
				depConns := getConnectors(dep)
				depConns.inCount++
				resConns.outs = append(resConns.outs, depConns.in)
			}
		}
	}
	return resourceConnectors
}

func (p *Plan) acceptDependencyInputs(
	resid string,
	conn *connectors) (ValidityTree, []Resource, bool) {
	// struct that will be passed to dependent graph node with validity data
	output := ValidityTree{
		ResourceID:     resid,
		ValidityStatus: Valid,
		Dependencies:   make([]ValidityTree, 0),
	}
	// Dependencies that changed for use in follow-on apply
	updatedDependencies := make([]Resource, 0)
	applyIfValid := false

	// process incoming data from each dependency; "worse" statuses override
	// less bad ones: Invalid > Inconclusive > Valid
	for i := 0; i < conn.inCount; i++ {
		vtree := <-conn.in
		// Add each incoming validity tree to the dependencies for the local one
		output.Dependencies = append(output.Dependencies, vtree)
		// Roll-up values; if all dependencies are valid, we move on to local
		// status checking
		switch vtree.ValidityStatus {
		case Invalid:
			output.ValidityStatus = Invalid
			output.Reason = DependencyInvalid
		case Inconclusive:
			if output.ValidityStatus != Invalid {
				output.ValidityStatus = Inconclusive
				output.Reason = DependencyInconclusive
			}
		case Valid:
			// If any dependency was "fixed" by an apply and indicated the downstream
			// resource should re-evaluate, we will re-apply the current resource
			if vtree.Updated {
				applyIfValid = true
				updatedDependencies = append(updatedDependencies, p.resources[vtree.ResourceID])
			}
		}
	}
	// Sort dependencies; produces deterministic output
	sortTreeDeps(output.Dependencies)
	return output, updatedDependencies, applyIfValid
}

// Start from independent resources and propagate validity status upward.
func (p *Plan) propagate(
	sortedResources []string,
	connectors map[string]*connectors,
	diff *Diff,
	runner Runner) error {
	for _, res := range sortedResources {
		if err := p.processResource(res, connectors, diff, runner); err != nil {
			return err
		}
	}
	return nil
}

// Logic here:
//
// If any dependency is invalid, propagate Invalid without
// retrying locally;
//
// otherwise, if any dependency is inconclusive, propagate Inconclusive
// without retrying locally
//
// If all dependencies are valid:
//   If any dependency said to update, re-apply the current resource
//   otherwise,
//     Check the current state (passed in or queried) against desired state
//     If invalid, re-apply the current resource
//        If apply fails, send/set invalid
//        otherwise, send/set valid with update value set to the propagate value
//          from the current resource
//     otherwise, send/set valid with 'false' for update
//   otherwise, send/set invalid
func (p *Plan) processResource(
	res string,
	conns map[string]*connectors,
	diff *Diff,
	runner Runner) error {
	logger := log.WithField("resource", res)
	conn := conns[res]
	output, updatedDependencies, applyIfValid := p.acceptDependencyInputs(res, conn)
	logger.Info("Applying")
	// function to pass data up the line to each dependent
	sendResults := func(output ValidityTree, conn *connectors) {
		for _, outchan := range conn.outs {
			outchan <- output
		}
	}
	// If any upstream resource was not valid, conclude this resource is not valid
	// and pass along the justification
	if output.ValidityStatus != Valid {
		logger.Error("Failing (Bad Upstream Resource)")
		sendResults(output, conn)
		return errors.New("Failing (Bad Upstream Resource)")
	}
	// if all upstream resources are valid, see if the state of the current
	// resource matches its desired state and, if not, re-apply the resource
	// and check again
	//
	// Any failure to query state results in an 'Inconclusive' status
	r := p.resources[res]
	var currentState State
	var stateRef interface{}
	if diff != nil {
		stateRef = diff.CurrentState[res]
	}
	if stateRef == nil {
		queriedState, err := r.QueryState(runner)
		if err != nil {
			output.ValidityStatus = Inconclusive
			output.Reason = QueryError
			output.ObservedError = err.Error()
			logger.Info("Failing (Bad Query)")
			sendResults(output, conn)
			return err
		}
		currentState = queriedState
	} else {
		currentState = stateRef.(State)
	}
	if applyIfValid || !reflect.DeepEqual(r.State(), currentState) {
		propagate, err := r.Apply(runner, Diff{currentState, updatedDependencies})
		if err != nil {
			// A failure to apply results in an 'Invalid' status
			output.ValidityStatus = Invalid
			output.Reason = ApplyError
			output.ObservedError = err.Error()
			// If the resource is a plan, grab its child validities
			if vt, ok := err.(ValidityTree); ok {
				output.Children = vt.Children
			}
			logger.Error("Failed")
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			return err
		} else {
			output.Updated = propagate
		}
	}
	logger.Debug("Finished")
	sendResults(output, conn)
	return nil
}
