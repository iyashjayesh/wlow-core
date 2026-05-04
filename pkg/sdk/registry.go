package sdk

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type ConfigField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
	EnvVar      string `json:"env_var,omitempty"`
}

type Definition struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Version      string        `json:"version"`
	Subjects     []string      `json:"subjects"`
	ConfigSchema []ConfigField `json:"config_schema,omitempty"`
	InputSchema  []SchemaField `json:"input_schema,omitempty"`
	OutputSchema []SchemaField `json:"output_schema,omitempty"`
	Factory      Factory       `json:"-"`
}

type Factory func(config map[string]any) (Handler, error)

type Registry struct {
	defs map[string]*Definition
}

func NewRegistry() *Registry {
	return &Registry{defs: make(map[string]*Definition)}
}

func (r *Registry) Register(def *Definition) error {
	if def.ID == "" {
		return fmt.Errorf("id required")
	}
	if def.Factory == nil {
		return fmt.Errorf("factory required")
	}
	if len(def.Subjects) == 0 {
		return fmt.Errorf("subjects required")
	}
	r.defs[def.ID] = def
	return nil
}

func (r *Registry) Get(id string) (*Definition, bool) {
	d, ok := r.defs[id]
	return d, ok
}

func (r *Registry) List() []*Definition {
	out := make([]*Definition, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	return out
}

func (r *Registry) Create(id string, config map[string]any) (Handler, error) {
	def, ok := r.defs[id]
	if !ok {
		return nil, fmt.Errorf("processor not found: %s", id)
	}

	cfg, err := r.applyDefaults(def, config)
	if err != nil {
		return nil, err
	}

	return def.Factory(cfg)
}

func (r *Registry) applyDefaults(def *Definition, config map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for _, f := range def.ConfigSchema {
		v, exists := config[f.Name]
		if !exists {
			if f.Required && f.Default == nil {
				return nil, fmt.Errorf("missing required: %s", f.Name)
			}
			if f.Default != nil {
				result[f.Name] = f.Default
			}
			continue
		}
		result[f.Name] = v
	}

	for k, v := range config {
		if _, exists := result[k]; !exists {
			result[k] = v
		}
	}

	return result, nil
}

func (def *Definition) JSON() ([]byte, error) {
	return json.Marshal(def)
}

// Builder

type Builder struct {
	def *Definition
}

func DefineProcessor(id string) *Builder {
	return &Builder{def: &Definition{ID: id}}
}

func (b *Builder) Name(n string) *Builder        { b.def.Name = n; return b }
func (b *Builder) Description(d string) *Builder { b.def.Description = d; return b }
func (b *Builder) Version(v string) *Builder     { b.def.Version = v; return b }
func (b *Builder) Subjects(s ...string) *Builder { b.def.Subjects = s; return b }

func (b *Builder) Config(name, typ, desc string, required bool, def any) *Builder {
	b.def.ConfigSchema = append(b.def.ConfigSchema, ConfigField{
		Name: name, Type: typ, Description: desc, Required: required, Default: def,
	})
	return b
}

func (b *Builder) ConfigEnv(name, typ, desc, env string, def any) *Builder {
	b.def.ConfigSchema = append(b.def.ConfigSchema, ConfigField{
		Name: name, Type: typ, Description: desc, EnvVar: env, Default: def,
	})
	return b
}

func (b *Builder) Input(name, typ, desc string, required bool) *Builder {
	b.def.InputSchema = append(b.def.InputSchema, SchemaField{
		Name: name, Type: typ, Description: desc, Required: required,
	})
	return b
}

func (b *Builder) Output(name, typ, desc string) *Builder {
	b.def.OutputSchema = append(b.def.OutputSchema, SchemaField{
		Name: name, Type: typ, Description: desc,
	})
	return b
}

func (b *Builder) Factory(f Factory) *Builder {
	b.def.Factory = f
	return b
}

func (b *Builder) Build() *Definition { return b.def }

func (b *Builder) MustRegister(r *Registry) *Definition {
	if err := r.Register(b.def); err != nil {
		panic(err)
	}
	return b.def
}

// Config helpers

type Configurable struct {
	cfg map[string]any
}

func (c *Configurable) SetConfig(cfg map[string]any) { c.cfg = cfg }
func (c *Configurable) Cfg() map[string]any          { return c.cfg }

func (c *Configurable) String(k string) string {
	if v, ok := c.cfg[k].(string); ok {
		return v
	}
	return ""
}

func (c *Configurable) Int(k string) int {
	switch v := c.cfg[k].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

func (c *Configurable) Bool(k string) bool {
	if v, ok := c.cfg[k].(bool); ok {
		return v
	}
	return false
}

func ParseConfig[T any](cfg map[string]any) (*T, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func ConfigSchemaFrom(v any) []ConfigField {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	var fields []ConfigField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := f.Tag.Get("json")
		if name == "" || name == "-" {
			continue
		}

		typ := "string"
		switch f.Type.Kind() {
		case reflect.Int, reflect.Int64:
			typ = "int"
		case reflect.Float64:
			typ = "float"
		case reflect.Bool:
			typ = "bool"
		case reflect.Slice:
			typ = "array"
		case reflect.Map, reflect.Struct:
			typ = "object"
		}

		_, hasDefault := f.Tag.Lookup("default")
		fields = append(fields, ConfigField{
			Name:        name,
			Type:        typ,
			Description: f.Tag.Get("desc"),
			Required:    !hasDefault,
		})
	}
	return fields
}
