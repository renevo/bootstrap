package config_test

import (
	"testing"

	"github.com/matryer/is"
	"github.com/renevo/config"
)

func TestNewSetCreatesIndependentRootSet(t *testing.T) {
	is := is.New(t)
	root := config.NewSet()
	is.True(root != nil)        // expected a new root set
	is.Equal(root.Path(), "")   // expected an empty root path
	is.Equal(root.Root(), root) // expected a new set to be its own root
}

func TestBindOnExplicitRootSetCreatesNestedSettings(t *testing.T) {
	is := is.New(t)
	root := config.NewSet()

	var cfg struct {
		Name string `setting:"name"`
		HTTP struct {
			Addr string `setting:"addr"`
		}
	}

	root.Bind(&cfg)

	nameSetting := root.Get("name")
	is.True(nameSetting != nil) // expected a top-level setting to be created

	httpSetting := root.Get("HTTP.addr")
	is.True(httpSetting != nil) // expected a nested setting to be created
}

func TestDefaultConvenienceSetStillWorks(t *testing.T) {
	is := is.New(t)
	oldDefault := config.Default
	config.Default = config.NewSet()
	t.Cleanup(func() {
		config.Default = oldDefault
	})

	setting := config.NewSetting("test", "value", "test setting")
	is.True(setting != nil)             // expected a setting from the convenience constructor
	is.Equal(setting.String(), "value") // expected the convenience constructor to use the provided value
}
