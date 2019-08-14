package object

// This type is used to make intent more clear when passing a simple string to something that expects an
// instance of Stringer

type String string

func (s String) String() string {
	return string(s)
}
