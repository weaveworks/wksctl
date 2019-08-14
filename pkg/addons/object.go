package addons

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/ghodss/yaml"
)

type object map[string]interface{}

var emptyObject = object(nil)

// newObject creates a new object akin to a json object.
func newObject() object {
	return make(map[string]interface{})
}

// newObjectFromJSON creates an object from JSON.
func newObjectFromJSON(r io.Reader) (object, error) {
	p := newObject()
	decoder := json.NewDecoder(r)
	err := decoder.Decode(&p)
	return p, err
}

// newObjectFromYAML creates an object from YAML.
func newObjectFromYAML(data []byte) (object, error) {
	json, err := yaml.YAMLToJSON(data)
	if err != nil {
		return emptyObject, err
	}
	r := bytes.NewReader(json)
	return newObjectFromJSON(r)
}

// IsEmpty returns true if the object doesn't contain any key.
func (o object) IsEmpty() bool {
	return len(o) == 0
}

// Equal returns true if two objects are equal.
func (o object) Equal(other object) bool {
	return reflect.DeepEqual(o, other)
}

// Get retrieves the value at from the object.
func (o object) Get(path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	m := o
	for i, part := range parts {
		v, found := m[part]
		if !found {
			return nil, fmt.Errorf("invalid path (key not found): %s", strings.Join(parts[:i+1], "."))
		}
		// Found the value!
		if i == len(parts)-1 {
			// When the value is a sub-object (as opposed to a primitive value), we box
			// it into a object to keep the nice property that a sub-tree of object is
			// still a object.
			if retMap, ok := v.(map[string]interface{}); ok {
				return object(retMap), nil
			}
			return v, nil
		}
		// We can only continue if we're traversing a map.
		if newMap, ok := v.(map[string]interface{}); ok {
			m = object(newMap)
			continue
		}
		return nil, fmt.Errorf("invalid path (key isn't a map): %s", strings.Join(parts[:i+1], "."))
	}

	// We shouldn't reach this.
	return nil, fmt.Errorf("invalid path (eek!): %s", path)
}

// GetBool retrieves a boolean value.
func (o object) GetBool(path string) (bool, error) {
	v, err := o.Get(path)
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

// Bool retrieves a boolean value.
func (o object) Bool(path string) bool {
	b, _ := o.GetBool(path)
	return b
}

// GetNumber retrieves a number value.
func (o object) GetNumber(path string) (float64, error) {
	v, err := o.Get(path)
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

// Number retrieves a number value.
func (o object) Number(path string) float64 {
	n, _ := o.GetNumber(path)
	return n
}

// GetString retrieves a string value.
func (o object) GetString(path string) (string, error) {
	v, err := o.Get(path)
	if err != nil {
		return "", err
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("cannot convert %q to string", v)
}

// String retrieves a string value.
func (o object) String(path string) string {
	str, _ := o.GetString(path)
	return str
}

// GetObjectArray retrieves an array of objects value.
func (o object) GetObjectArray(path string) ([]object, error) {
	v, err := o.Get(path)
	if err != nil {
		return nil, err
	}
	if reflect.TypeOf(v).Kind() != reflect.Slice {
		return nil, fmt.Errorf("not a slice: %q", v)
	}
	slice := reflect.ValueOf(v)

	var objects []object
	for i := 0; i < slice.Len(); i++ {
		if elem, ok := slice.Index(i).Interface().(map[string]interface{}); ok {
			objects = append(objects, elem)
		} else {
			return nil, fmt.Errorf("cannot convert %q to object", slice.Index(i))
		}
	}

	return objects, nil
}

// ObjectArray retrieves an array value.
func (o object) ObjectArray(path string) []object {
	a, _ := o.GetObjectArray(path)
	return a
}

// GetObject retrieves an object value.
func (o object) GetObject(path string) (object, error) {
	v, err := o.Get(path)
	if err != nil {
		return newObject(), err
	}
	if o, ok := v.(object); ok {
		return o, nil
	}
	return newObject(), fmt.Errorf("cannot convert %q to object", v)
}

// Object retrieves an object parameter.
func (o object) Object(path string) object {
	v, _ := o.GetObject(path)
	return v
}

func isMap(v interface{}) bool {
	_, ok := v.(map[string]interface{})
	return ok
}

// Set sets a property.
func (o object) Set(path string, v interface{}) {
	parts := strings.Split(path, ".")
	m := o
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

	// Internal types are only primitive types and maps, not object. Convert object
	// to map[string]interface{}.
	if p, ok := v.(object); ok {
		v = map[string]interface{}(p)
	}

	key := parts[len(parts)-1]
	m[key] = v
}

// SetBool sets a boolean property.
func (o object) SetBool(path string, b bool) {
	o.Set(path, b)
}

// SetNumber sets a number property.
func (o object) SetNumber(path string, f float64) {
	o.Set(path, f)
}

// SetString sets a string property.
func (o object) SetString(path string, str string) {
	o.Set(path, str)
}

// SetObject sets an object property.
func (o object) SetObject(path string, obj object) {
	o.Set(path, obj)
}

// Merge merges two objects.
func (o object) Merge(a object) {
	for k, v := range a {
		// k isn't in the original map, set it.
		if _, ok := o[k]; !ok {
			o[k] = v
			continue
		}
		dst := o[k]

		// Both original and incoming values are maps.
		dstMap, dstOk := dst.(map[string]interface{})
		srcMap, srcOk := v.(map[string]interface{})
		if dstOk && srcOk {
			object(dstMap).Merge(srcMap)
			continue
		}

		// Otherwise, source overrides destination
		o[k] = v
	}
}

// toJSON returns a JSON representation of the object.
func (o object) toJSON() ([]byte, error) {
	return json.MarshalIndent(o, "", " ")
}

// toYAML returns a YAML representation of the object.
func (o object) toYAML() ([]byte, error) {
	return yaml.Marshal(o)
}
