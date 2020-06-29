package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/chanwit/plandiff"
)

// State is a collection of (key, value) pairs describing unequivocally a step
// and, by extension, a plan.
type State map[string]interface{}

// EmptyState is the empty State.
var EmptyState = State(nil)

// NewState creates an empty set of parameters.
func NewState() State {
	return make(map[string]interface{})
}

// NewStateFromJSON creates State from JSON.
func NewStateFromJSON(r io.Reader) (State, error) {
	p := NewState()
	decoder := json.NewDecoder(r)
	err := decoder.Decode(&p)
	return p, err
}

// Turn a State representation of a Plan into a Plan (used in Plan
// serialization)
func (s State) toPlan() Plan {
	p := newPlan()
	// Note: not using builder because the nodes do not necessarily come back in
	// their original order and adding dependency edges prior to their nodes
	// is not supported by the graph. So, we add all nodes and then all edges
	for id := range s {
		p.graph.AddNode(id)
	}
	for id, dval := range s {
		data := dval.(map[string]interface{})

		// First the graph
		// When we go through JSON, this comes back as []interface{}; otherwise it's []string
		depsint := (data["meta"].(map[string]interface{}))["dependsOn"]
		var deps []string
		if depstrs, ok := depsint.([]string); ok {
			deps = depstrs
		} else {
			for _, str := range depsint.([]interface{}) {
				deps = append(deps, str.(string))
			}
		}
		for _, dep := range deps {
			p.graph.AddEdge(dep, id)
		}

		// Now the resources
		var rtype resourceType
		var vals map[string]interface{}
		// One of the entries in the map is the metadata (labeled
		// "meta"); the other is the Resource but we don't know its
		// name since it's named by its type
		for k, v := range data {
			if k != "meta" {
				rtype = resourceTypeByName(k)
				vals = v.(map[string]interface{})
				break
			}
		}
		// If the resource is a nested Plan, just use the "toPlan" method
		if rtype.baseType == reflect.ValueOf(Plan{}).Type() {
			pres := State(vals).toPlan()
			p.resources[id] = &pres
			continue
		}
		// Create a new instance of the Resource's struct type and map
		// tagged names to actual struct field names
		resref := reflect.New(rtype.baseType).Elem()
		fnameMap := extractActualNamesFromTags(rtype.baseType)

		// Set the fields to the loaded values
		for fname, fval := range vals {
			if fval != nil {
				resref.FieldByName(lookupActualName(fnameMap, fname)).Set(reflect.ValueOf(fval))
			}
		}

		// If the original Resource was implemented as a pointer type,
		// take the address before turning the value into a Resource
		var res Resource
		if rtype.isPtr {
			res = resref.Addr().Interface().(Resource)
		} else {
			res = resref.Interface().(Resource)
		}
		p.resources[id] = res
	}
	return *p
}

// Since we're using "structs" to turn our Resource structures into maps, we need to
// pull the "structs" tags to find the actual field names
func extractActualNamesFromTags(restype reflect.Type) map[string]string {
	result := make(map[string]string)
	for i := 0; i < restype.NumField(); i++ {
		f := restype.Field(i)
		structsTag := f.Tag.Get("structs")
		if structsTag != "" {
			result[strings.Split(structsTag, ",")[0]] = f.Name
		}
	}
	return result
}

// Return the real field name if a "structs"-defined name was serialized
func lookupActualName(fnameMap map[string]string, fname string) string {
	if actualName, ok := fnameMap[fname]; ok {
		return actualName
	}
	return fname
}

// IsEmpty returns true if the state doesn't contain any key.
func (s State) IsEmpty() bool {
	return len(s) == 0
}

// Equal returns true if two States are equal.
func (s State) Equal(other State) bool {
	return reflect.DeepEqual(s, other)
}

// Get retrieves a parameter.
func (s State) Get(path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	m := s
	for i, part := range parts {
		v, found := m[part]
		if !found {
			return nil, fmt.Errorf("invalid path (key not found): %s", strings.Join(parts[:i+1], "."))
		}
		// Found the value!
		if i == len(parts)-1 {
			// When the value is a sub-object (as opposed to a primitive value), we box
			// it into a State to keep the nice property that a sub-tree of State is
			// still a State.
			if retMap, ok := v.(map[string]interface{}); ok {
				return State(retMap), nil
			}
			return v, nil
		}
		// We can only continue if we're traversing a map.
		if newMap, ok := v.(map[string]interface{}); ok {
			m = State(newMap)
			continue
		}
		return nil, fmt.Errorf("invalid path (key isn't a map): %s", strings.Join(parts[:i+1], "."))
	}

	// We shouldn't reach this.
	return nil, fmt.Errorf("invalid path (eek!): %s", path)
}

// GetBool retrieves a boolean parameter.
func (s State) GetBool(path string) (bool, error) {
	v, err := s.Get(path)
	if err != nil {
		return false, err
	}
	if b, ok := v.(bool); ok {
		return b, nil
	}
	// string -> bool coercion
	if s, ok := v.(string); ok {
		if b, err := strconv.ParseBool(s); err == nil {
			return b, nil
		}
	}
	return false, fmt.Errorf("cannot convert %q to bool", v)
}

// Bool retrieves a boolean parameter.
func (s State) Bool(path string) bool {
	b, _ := s.GetBool(path)
	return b
}

// GetNumber retrieves a number parameter.
func (s State) GetNumber(path string) (float64, error) {
	v, err := s.Get(path)
	if err != nil {
		return 0, err
	}
	if f, ok := v.(float64); ok {
		return f, nil
	}
	// string -> number coercion.
	if s, ok := v.(string); ok {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, nil
		}
	}
	return 0, fmt.Errorf("cannot convert %q to float64", v)
}

// Number retrieves a number parameter.
func (s State) Number(path string) float64 {
	n, _ := s.GetNumber(path)
	return n
}

// GetString retrieves a string parameter.
func (s State) GetString(path string) (string, error) {
	v, err := s.Get(path)
	if err != nil {
		return "", err
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("cannot convert %q to string", v)
}

// String retrieves a string parameter.
func (s State) String(path string) string {
	str, _ := s.GetString(path)
	return str
}

// GetObject retrieves a object property.
func (s State) GetObject(path string) (State, error) {
	v, err := s.Get(path)
	if err != nil {
		return NewState(), err
	}
	if o, ok := v.(State); ok {
		return o, nil
	}
	return NewState(), fmt.Errorf("cannot convert %q to State", v)
}

// Object retrieves an object parameter.
func (s State) Object(path string) State {
	v, _ := s.GetObject(path)
	return v
}

func isMap(v interface{}) bool {
	_, ok := v.(map[string]interface{})
	return ok
}

// Set sets a property.
func (s State) Set(path string, v interface{}) {
	parts := strings.Split(path, ".")
	m := s
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		value, ok := m[part]
		// If the part key doesn't exist yet or is primitive type, create it.
		if !ok || !isMap(value) {
			p := make(map[string]interface{})
			m[part] = p
			m = p
			continue
		}
		// Continue through the chain of maps
		m = m[part].(map[string]interface{})
	}

	// Internal types are only primitive types and maps, not State. Convert State
	// to map[string]interface{}.
	if p, ok := v.(State); ok {
		v = map[string]interface{}(p)
	}

	key := parts[len(parts)-1]
	m[key] = v
}

// SetBool sets a boolean property.
func (s State) SetBool(path string, b bool) {
	s.Set(path, b)
}

// SetNumber sets a number property.
func (s State) SetNumber(path string, f float64) {
	s.Set(path, f)
}

// SetString sets a string property.
func (s State) SetString(path string, str string) {
	s.Set(path, str)
}

// SetObject sets an object property.
func (s State) SetObject(path string, o State) {
	s.Set(path, o)
}

// Merge merges two States.
func (s State) Merge(a State) {
	for k, v := range a {
		// k isn't in the original map, set it.
		if _, ok := s[k]; !ok {
			s[k] = v
			continue
		}
		dst := s[k]

		// Both original and incoming values are maps.
		dstMap, dstOk := dst.(map[string]interface{})
		srcMap, srcOk := v.(map[string]interface{})
		if dstOk && srcOk {
			State(dstMap).Merge(srcMap)
			continue
		}

		// Otherwise, source overrides destination
		s[k] = v
	}
}

// Diff returns human-readable diffs from one State to another.
func (s State) Diff(t State) (string, error) {
	strA, _ := json.MarshalIndent(s, "", " ")
	strB, _ := json.MarshalIndent(t, "", " ")
	return plandiff.GetUnifiedDiff(string(strA), string(strB))
}
