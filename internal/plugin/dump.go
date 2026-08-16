package plugin

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// DumpRow is one composed row, for dsh --dump-config style inspection.
type DumpRow struct {
	ID       string   `yaml:"id" json:"id"`
	Plugin   string   `yaml:"plugin" json:"plugin"`
	Disabled bool     `yaml:"disabled,omitempty" json:"disabled,omitempty"`
	Inject   []string `yaml:"inject,omitempty" json:"inject,omitempty"`
}

// Dump returns YAML of the mounted tree. Every row can be targeted by a patch.
func (h *Host) Dump() string {
	rows := make([]DumpRow, 0, len(h.mounted))
	for _, m := range h.mounted {
		rows = append(rows, DumpRow{
			ID: m.ID, Plugin: m.Plugin, Disabled: m.Disabled, Inject: m.Inject,
		})
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(rows); err != nil {
		return fmt.Sprintf("# dump error: %v\n", err)
	}
	_ = enc.Close()
	return buf.String()
}
