package instances

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// Instance represents a named OpenObserve instance.
type Instance struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	Org  string `yaml:"org"`
	User string `yaml:"user"`
}

// instancesFile is the persisted YAML document.
type instancesFile struct {
	Instances []Instance `yaml:"instances"`
	Current   string     `yaml:"current,omitempty"`
}

// Manager persists and queries named instances.
type Manager struct {
	instances   map[string]Instance
	currentName string
	configPath  string
}

// NewManager returns a Manager that reads/writes instances to the given path.
// The file is created with an empty document if it does not exist.
func NewManager(configPath string) (*Manager, error) {
	m := &Manager{
		instances:   make(map[string]Instance),
		configPath:  configPath,
		currentName: "",
	}
	if err := m.load(); err != nil {
		return nil, err
	}

	return m, nil
}

// Add registers a new instance. It returns an error if the name already exists.
func (m *Manager) Add(name, url, org, user string) error {
	if _, ok := m.instances[name]; ok {
		return fmt.Errorf("instance %q already exists", name)
	}
	m.instances[name] = Instance{Name: name, URL: url, Org: org, User: user}

	return m.save()
}

// Remove deletes an instance by name. It returns an error if the name does not exist.
func (m *Manager) Remove(name string) error {
	if _, ok := m.instances[name]; !ok {
		return fmt.Errorf("instance %q not found", name)
	}
	delete(m.instances, name)
	if m.currentName == name {
		m.currentName = ""
	}

	return m.save()
}

// Get returns the instance with the given name. It returns an error if not found.
func (m *Manager) Get(name string) (Instance, error) {
	inst, ok := m.instances[name]
	if !ok {
		return Instance{}, fmt.Errorf("instance %q not found. Run 'o2 instance add %s ...' to add it", name, name)
	}

	return inst, nil
}

// SetCurrent sets the default instance. It returns an error if the name does not exist.
func (m *Manager) SetCurrent(name string) error {
	if _, ok := m.instances[name]; !ok {
		return fmt.Errorf("instance %q not found", name)
	}
	m.currentName = name

	return m.save()
}

// Current returns the current default instance and whether a default is set.
func (m *Manager) Current() (Instance, bool, error) {
	if m.currentName == "" {
		return Instance{}, false, nil
	}
	inst, ok := m.instances[m.currentName]
	if !ok {
		// Current was set but the instance was deleted — clear it.
		m.currentName = ""
		_ = m.save()

		return Instance{}, false, nil
	}

	return inst, true, nil
}

// List returns all registered instances in insertion order.
func (m *Manager) List() []Instance {
	out := make([]Instance, 0, len(m.instances))
	for _, name := range sortedKeys(m.instances) {
		out = append(out, m.instances[name])
	}

	return out
}

// load reads the YAML file from disk, overwriting any in-memory state.
func (m *Manager) load() error {
	content, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("reading instances file: %w", err)
	}
	var f instancesFile
	if err := yaml.Unmarshal(content, &f); err != nil {
		return fmt.Errorf("parsing instances file: %w", err)
	}
	m.instances = make(map[string]Instance)
	for _, inst := range f.Instances {
		m.instances[inst.Name] = inst
	}
	m.currentName = f.Current

	return nil
}

// save writes the current in-memory state to the YAML file.
func (m *Manager) save() error {
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	insts := m.List()
	f := instancesFile{
		Instances: insts,
		Current:   m.currentName,
	}
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("encoding instances file: %w", err)
	}

	return os.WriteFile(m.configPath, data, 0o600)
}

func sortedKeys(m map[string]Instance) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	return keys
}
