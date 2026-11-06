package config

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
)

// Examples document optional file mappings and named sections without enabling them.
// Values are serialized by the same encoder as actual defaults.
var configExamples = map[reflect.Type]any{
	reflect.TypeFor[PortSettings](): PortSettings{HostPort: 3000, VMPort: 8080},
	reflect.TypeFor[FileSettings](): FileSettings{
		Source: "~/.config/opencode/skills", Target: "/root/.config/opencode/skills", Mode: "mount",
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

type configField struct {
	name, path, description string
	value                   reflect.Value
}

func writeConfigTable(out *bytes.Buffer, value reflect.Value, path string, commented bool) error {
	typ := value.Type()
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
		entry := configField{name: name, path: fieldPath, description: description, value: value.Field(i)}
		if err := entry.write(out, commented); err != nil {
			return err
		}
	}
	return nil
}

func (f configField) write(out *bytes.Buffer, commented bool) error {
	prefix := ""
	if commented {
		prefix = "# "
	}
	if f.value.Kind() == reflect.Slice && f.value.Type().Elem().Kind() == reflect.Struct {
		return f.writeExample(out, prefix)
	}
	switch f.value.Kind() {
	case reflect.Pointer:
		fmt.Fprintf(out, "# %s\n# %s = {}\n", f.description, f.name)
	case reflect.Struct:
		fmt.Fprintf(out, "\n# %s\n%s[%s]\n", f.description, prefix, f.path)
		return writeConfigTable(out, f.value, f.path, commented)
	case reflect.Map:
		return f.writeExample(out, prefix)
	default:
		fmt.Fprintf(out, "# %s\n", f.description)
		var encoded bytes.Buffer
		if err := toml.NewEncoder(&encoded).Encode(map[string]any{f.name: f.value.Interface()}); err != nil {
			return err
		}
		for _, line := range strings.Split(strings.TrimSuffix(encoded.String(), "\n"), "\n") {
			fmt.Fprintf(out, "%s%s\n", prefix, line)
		}
	}
	return nil
}

func (f configField) writeExample(out *bytes.Buffer, prefix string) error {
	example, ok := configExamples[f.value.Type().Elem()]
	if !ok {
		return fmt.Errorf("config section %s requires an example", f.path)
	}
	path := f.path
	if f.value.Kind() == reflect.Slice {
		fmt.Fprintf(out, "# %s\n%s%s = []\n\n# Remove the empty list above to enable this example.\n# [[%s]]\n", f.description, prefix, f.name, path)
	} else {
		path += ".example"
		fmt.Fprintf(out, "\n# %s\n# [%s]\n", f.description, path)
	}
	return writeConfigTable(out, reflect.ValueOf(example), path, true)
}
