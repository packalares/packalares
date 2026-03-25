package appservice

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

// injectValuesYaml reads the chart's values.yaml, deep-merges the provided
// overrides into it (without overwriting existing values), and writes the
// result back. This replaces the old approach of passing --set flags to helm,
// which failed because every chart has different value paths and nested
// structures that --set cannot express correctly.
func injectValuesYaml(chartDir string, overrides map[string]interface{}) error {
	valuesPath := filepath.Join(chartDir, "values.yaml")

	existing := make(map[string]interface{})

	data, err := os.ReadFile(valuesPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read values.yaml: %w", err)
		}
		// No values.yaml yet -- we will create one
		klog.V(2).Infof("no values.yaml in %s, creating one", chartDir)
	} else {
		if err := yaml.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parse values.yaml: %w", err)
		}
		if existing == nil {
			existing = make(map[string]interface{})
		}
	}

	// Deep-merge: overrides fill in missing keys but do NOT overwrite
	// values that already exist in the chart's values.yaml.
	deepMergeUnder(existing, overrides)

	out, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal values.yaml: %w", err)
	}

	if err := os.WriteFile(valuesPath, out, 0644); err != nil {
		return fmt.Errorf("write values.yaml: %w", err)
	}

	klog.V(2).Infof("injected %d top-level keys into %s", len(overrides), valuesPath)
	return nil
}

// deepMergeUnder merges src into dst. For each key in src:
//   - if the key does not exist in dst, it is added
//   - if both values are maps, recurse
//   - if the key already exists in dst and is not a map, keep the dst value
//     (i.e. the chart's original value wins)
//
// This means the chart author's defaults are preserved, while we fill in
// platform-level values that the chart expects but leaves empty (e.g.
// postgres.host, redis.password, bfl.username).
func deepMergeUnder(dst, src map[string]interface{}) {
	for key, srcVal := range src {
		dstVal, exists := dst[key]
		if !exists {
			// Key missing in dst -- add it wholesale
			dst[key] = srcVal
			continue
		}

		// Both exist -- if both are maps, recurse
		srcMap, srcIsMap := toStringMap(srcVal)
		dstMap, dstIsMap := toStringMap(dstVal)

		if srcIsMap && dstIsMap {
			deepMergeUnder(dstMap, srcMap)
			dst[key] = dstMap
		}
		// else: dst already has a non-map value, keep it
	}
}

// toStringMap attempts to convert a value to map[string]interface{}.
// YAML unmarshalling produces map[string]interface{} but we handle both
// that and the less common map[interface{}]interface{} from older parsers.
func toStringMap(v interface{}) (map[string]interface{}, bool) {
	switch m := v.(type) {
	case map[string]interface{}:
		return m, true
	case map[interface{}]interface{}:
		result := make(map[string]interface{}, len(m))
		for k, val := range m {
			result[fmt.Sprintf("%v", k)] = val
		}
		return result, true
	default:
		return nil, false
	}
}

// restructureSubcharts moves subdirectories that contain Chart.yaml into a
// charts/ subdirectory. Olares apps put subcharts at root level (e.g.
// myapp/myapp/, myapp/myappserver/) but Helm expects them under charts/.
func restructureSubcharts(chartDir string) {
	entries, err := os.ReadDir(chartDir)
	if err != nil {
		return
	}

	var subcharts []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip known non-subchart directories
		if name == "templates" || name == "charts" || name == "crds" || name == "i18n" || name == "ci" {
			continue
		}
		// Check if this directory has its own Chart.yaml
		subChartYaml := filepath.Join(chartDir, name, "Chart.yaml")
		if _, err := os.Stat(subChartYaml); err == nil {
			subcharts = append(subcharts, name)
		}
	}

	if len(subcharts) == 0 {
		return
	}

	// Create charts/ directory
	chartsSubdir := filepath.Join(chartDir, "charts")
	if err := os.MkdirAll(chartsSubdir, 0755); err != nil {
		klog.Warningf("create charts/ dir: %v", err)
		return
	}

	// Move each subchart
	for _, name := range subcharts {
		src := filepath.Join(chartDir, name)
		dst := filepath.Join(chartsSubdir, name)
		if err := os.Rename(src, dst); err != nil {
			klog.Warningf("move subchart %s to charts/: %v", name, err)
		} else {
			klog.Infof("moved subchart %s → charts/%s", name, name)
		}
	}
}
