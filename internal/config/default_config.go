package config

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
)

// Examples document optional mounts and named sections without enabling them.
// Values are serialized by the same encoder as actual defaults.
var configExamples = map[reflect.Type]any{
	reflect.TypeFor[MountSettings](): MountSettings{
		Source: "~/.config/opencode/skills", Target: "/root/.config/opencode/skills",
	},
	reflect.TypeFor[SecretSettings](): SecretSettings{
		File: "creds/token", Env: "API_TOKEN",
		AllowedHosts: []string{"api.example.com"}, Headers: []string{"authorization"},
	},
}

// WriteDefaultConfig prints documented built-in defaults, without reading any
// user configuration, trust state, or credentials. Optional examples are inert.
func WriteDefaultConfig(w io.Writer) error {
	var out bytes.Buffer
	fmt.Fprintln(&out, "# Launcher defaults. Global config and approved workspace config override these values.")
	if err := writeConfigTable(&out, reflect.ValueOf(DefaultSettings()), "", false); err != nil {
		return err
	}
	_, err := w.Write(out.Bytes())
	return err
}

func writeConfigTable(out *bytes.Buffer, value reflect.Value, path string, commented bool) error {
	typ := value.Type()
	prefix := ""
	if commented {
		prefix = "# "
	}
	for i := 0; i < value.NumField(); i++ {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("toml"), ",")[0]
		if name == "-" {
			continue
		}
		description := strings.TrimSpace(field.Tag.Get("description"))
		if name == "" || description == "" {
			return fmt.Errorf("config field %s.%s requires TOML name and description tags", typ.Name(), field.Name)
		}
		fieldPath := name
		if path != "" {
			fieldPath = path + "." + name
		}
		fieldValue := value.Field(i)
		if fieldValue.Kind() == reflect.Slice && fieldValue.Type().Elem().Kind() == reflect.Struct {
			example, ok := configExamples[fieldValue.Type().Elem()]
			if !ok {
				return fmt.Errorf("config section %s requires an example", fieldPath)
			}
			fmt.Fprintf(out, "# %s\n%s%s = []\n\n# Remove the empty list above to enable this example.\n# [[%s]]\n", description, prefix, name, fieldPath)
			if err := writeConfigTable(out, reflect.ValueOf(example), fieldPath, true); err != nil {
				return err
			}
			continue
		}
		switch fieldValue.Kind() {
		case reflect.Pointer:
			fmt.Fprintf(out, "# %s\n# %s = {}\n", description, name)
		case reflect.Struct:
			fmt.Fprintf(out, "\n# %s\n%s[%s]\n", description, prefix, fieldPath)
			if err := writeConfigTable(out, fieldValue, fieldPath, commented); err != nil {
				return err
			}
		case reflect.Map:
			example, ok := configExamples[fieldValue.Type().Elem()]
			if !ok {
				return fmt.Errorf("config section %s requires an example", fieldPath)
			}
			fmt.Fprintf(out, "\n# %s\n# [%s.example]\n", description, fieldPath)
			if err := writeConfigTable(out, reflect.ValueOf(example), fieldPath+".example", true); err != nil {
				return err
			}
		default:
			fmt.Fprintf(out, "# %s\n", description)
			var encoded bytes.Buffer
			if err := toml.NewEncoder(&encoded).Encode(map[string]any{name: fieldValue.Interface()}); err != nil {
				return err
			}
			for _, line := range strings.Split(strings.TrimSuffix(encoded.String(), "\n"), "\n") {
				fmt.Fprintf(out, "%s%s\n", prefix, line)
			}
		}
	}
	return nil
}
