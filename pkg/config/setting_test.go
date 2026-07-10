package config

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/matryer/is"
)

type setTest struct {
	To          any
	Initializer any
	CheckString string
}

func (s setTest) Value() any {
	return reflect.Indirect(reflect.ValueOf(s.Initializer)).Interface()
}

func (s setTest) Equals(v any) bool {
	return reflect.Indirect(reflect.ValueOf(s.Initializer)).Interface() == v
}

func newSetTest(to any, v any, s string) setTest {
	p := reflect.New(reflect.TypeOf(v))
	p.Elem().Set(reflect.ValueOf(v))

	return setTest{
		To:          to,
		Initializer: p.Interface(),
		CheckString: s,
	}
}

func TestSetting_Set(t *testing.T) {
	tests := []setTest{
		newSetTest("changed", "initial", "initial"),
		newSetTest(true, false, "False"),
		newSetTest(time.Minute*23, time.Minute, "1m"),

		newSetTest(int(23), int(5), "5"),
		newSetTest(int8(23), int8(5), "5"),
		newSetTest(int16(23), int16(5), "5"),
		newSetTest(int32(23), int32(5), "5"),
		newSetTest(int64(23), int64(5), "5"),

		newSetTest(uint(23), uint(5), "5"),
		newSetTest(uint8(23), uint8(5), "5"),
		newSetTest(uint16(23), uint16(5), "5"),
		newSetTest(uint32(23), uint32(5), "5"),
		newSetTest(uint64(23), uint64(5), "5"),

		newSetTest(float32(23), float32(5), "5"),
		newSetTest(float64(23), float64(5), "5"),

		// actually treated like a uint8, but we make sure it works
		newSetTest(byte(6), byte(5), "5"),
	}

	for _, test := range tests {
		testName := fmt.Sprintf("%T", test.To)

		// override the internal type resolutions to test type aliases
		if v, ok := test.To.(byte); ok && v == 6 {
			testName = "byte"
		}

		t.Run(testName, func(t *testing.T) {
			is := is.New(t)
			s := &Setting{Value: test.Initializer}

			// validates if the provided string matches the formatting of the string value
			is.True(s.Equals(test.CheckString)) // expected the initial string value to match the setting string

			// make sure we can set the value from the provided string value
			err := s.Set(test.CheckString)
			is.NoErr(err) // expected setting updates from string values to succeed

			// make sure we can set the value from the raw sprinted to string
			err = s.Set(fmt.Sprintf("%v", test.To))
			is.NoErr(err) // expected setting updates from formatted values to succeed

			// validate that the pointer was in fact changed to the new value
			is.True(test.Equals(test.To)) // expected the underlying value to be updated to the new target value

			// validate that we don't get a blank string back (could probably be a better test TBH)
			is.True(s.String() != "") // expected the string value to be non-empty

			// validate that the fmt.sprintf matches the equality checker
			is.True(s.Equals(fmt.Sprintf("%v", test.To))) // expected the formatted value to match the setting equality logic
		})
	}
}

type customSetting struct {
	Value       []byte
	Marshaled   bool
	Unmarshaled bool
	Equaled     bool
}

func (cs *customSetting) UnmarshalSetting(v string) error {
	cs.Value = []byte(v)
	cs.Unmarshaled = true
	return nil
}

func (cs *customSetting) MarshalSetting() string {
	cs.Marshaled = true
	return string(cs.Value)
}

func (cs *customSetting) Equals(v string) bool {
	cs.Equaled = true
	return bytes.Equal(cs.Value, []byte(v))
}

func TestSetting_CustomType(t *testing.T) {
	is := is.New(t)
	cs := &customSetting{
		Value: []byte("hello"),
	}

	st := &Setting{Value: cs}

	is.Equal(string(cs.Value), st.String()) // expected a custom type to stringify through MarshalSetting
	is.True(cs.Marshaled)                   // expected MarshalSetting to be called for custom types

	newValue := "goodbye"

	err := st.Set(newValue)
	is.NoErr(err)                        // expected custom values to be set successfully
	is.True(cs.Unmarshaled)              // expected UnmarshalSetting to be called for custom types
	is.Equal(string(cs.Value), newValue) // expected the custom value to be updated
	is.True(st.Equals(newValue))         // expected the setting to equal the updated custom value
	is.True(cs.Equaled)                  // expected Equals to be called for custom types
}

func TestSetting_Notify(t *testing.T) {
	is := is.New(t)
	name := "Test"
	value1 := "value1"
	value2 := "value2"

	st := &Setting{Name: name, Value: value1}

	notifyCalled := false
	nh := st.Notify(NotifyFunc(func(s *Setting) {
		if s.Name != name {
			t.Errorf("Notification Setting Name did not Match expected name; expected %q got %q", name, s.Name)
		}
		notifyCalled = true
	}))

	err := st.Set(value1)
	is.NoErr(err)          // expected the initial setting update to succeed
	is.True(!notifyCalled) // expected no notification when the value stays the same
	notifyCalled = false

	err = st.Set(value2)
	is.NoErr(err)         // expected the changed setting update to succeed
	is.True(notifyCalled) // expected a notification when the value changes
	notifyCalled = false

	err = nh.Close()
	is.NoErr(err) // expected the notification handle to close successfully

	err = st.Set(value1)
	is.NoErr(err)          // expected a later setting update to succeed
	is.True(!notifyCalled) // expected no notification after the handle has been closed

}

func TestSetting_FlagCompat(t *testing.T) {
	is := is.New(t)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	st := &Setting{Name: "debug", Description: "Sets debug mode", Value: false}
	st.Flag("debug", fs)

	err := fs.Parse([]string{"-debug"})
	is.NoErr(err)               // expected the debug flag to parse successfully
	is.True(st.Value.(bool))    // expected the bool setting to be updated by the flag
	is.Equal(st.Type(), "bool") // expected the flag-compatible type to resolve as bool
}

type errWriter struct {
	err error
}

func (w errWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

func TestSetDumpReturnsWriteError(t *testing.T) {
	is := is.New(t)
	set := NewSet()
	set.Setting("name", "value", "description")

	expectedErr := errors.New("write failed")
	err := set.Dump(errWriter{err: expectedErr})
	is.True(errors.Is(err, expectedErr)) // expected Dump to surface the underlying write error
}
