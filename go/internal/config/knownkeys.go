package config

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// unknownKey is one key the loaded schema does not declare, together with the
// source line so the message can point at a single place to fix.
type unknownKey struct {
	Path string
	Line int
}

// unknownKeys walks the YAML document and collects every key that Config /
// ModelConfig / every nested struct does not declare, using the path where that
// key is actually READ — for a key inherited through `extends` that is the
// inheriting entry, not the base.
//
// Why this exists: LoadConfig cannot decode strictly (a legacy `ai:` block and a
// few lenient value shapes must keep working) while a TYPO'D KEY is the one
// config error with no symptom at all — the value simply never arrives and the
// run quietly uses the default. `docvision setup` catches the entry the key is
// written in; this warning additionally names the entries that inherited it.
func unknownKeys(data []byte) ([]unknownKey, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	root := documentRoot(&doc)
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, nil
	}
	seen := map[string]unknownKey{}
	// extendsOf mirrors merge.go: it lets a warning name the entries that
	// inherited a bad key even though the merged document no longer holds it.
	extendsOf := map[string][]string{}
	if models := mapNodeValue(root, modelsKey); models != nil && models.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(models.Content); i += 2 {
			entry := models.Content[i+1]
			if entry.Kind != yaml.MappingNode {
				continue
			}
			if parents, ok, err := extendsRefs(models.Content[i].Value, entry); err == nil && ok {
				for _, parent := range parents {
					extendsOf[parent] = append(extendsOf[parent], models.Content[i].Value)
				}
			}
		}
	}
	walkUnknownKeys(root, "", &Config{}, extendsOf, seen)
	out := make([]unknownKey, 0, len(seen))
	for _, k := range seen {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

// walkUnknownKeys descends a mapping node alongside the value that should
// describe it: a struct (its declared fields are the known keys) or a map (any
// key is known, the element struct describes the value). An empty path means
// "the document root".
func walkUnknownKeys(node *yaml.Node, path string, target interface{}, extendsOf map[string][]string, found map[string]unknownKey) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	vt := derefType(reflect.TypeOf(target))
	if vt == nil {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		child := joinPath(path, key.Value)
		if vt.Kind() == reflect.Map {
			// Free-form keys (models.<条目>, request_body, thinking): every key
			// is legal, the element struct only describes the value.
			walkUnknownKeys(value, child, newElem(vt.Elem()), extendsOf, found)
			continue
		}
		next := structField(vt, key.Value)
		if next == nil {
			reportUnknownKey(child, key, extendsOf, found)
			continue // cannot descend into an undeclared subtree
		}
		walkUnknownKeys(value, child, next, extendsOf, found)
	}
}

// reportUnknownKey records an undeclared key, plus the inheriting paths when the
// key sits in a models entry that other entries extend.
func reportUnknownKey(child string, key *yaml.Node, extendsOf map[string][]string, found map[string]unknownKey) {
	paths := []string{child}
	if base := strings.TrimSuffix(child, "."+key.Value); strings.HasPrefix(base, modelsKey+".") {
		for _, inheritor := range extendsOf[strings.TrimPrefix(base, modelsKey+".")] {
			paths = append(paths, modelsKey+"."+inheritor+"."+key.Value)
		}
	}
	for _, p := range paths {
		if prev, ok := found[p]; !ok || key.Line < prev.Line {
			found[p] = unknownKey{Path: p, Line: key.Line}
		}
	}
}

// joinPath builds the dotted config path used in every report.
func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// structField returns a value describing the YAML key `name`, or nil when no
// struct tag declares it. It follows yaml.v3's own rules: exact tag match (a
// bare field name is its lowercase form), inline embedded structs are
// flattened, and `yaml:"-"` is not a config key. A path-like tag
// (`yaml:"chapters/<base>.tex"`) is not matchable, which is correct — no
// document contains such a key.
func structField(t reflect.Type, name string) interface{} {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		tag, hasTag := f.Tag.Lookup("yaml")
		tagName := ""
		if hasTag {
			tagName = strings.Split(tag, ",")[0]
			if tagName == "-" {
				continue
			}
		}
		if tagName == "" && f.Anonymous {
			// Inline embedded struct: its fields belong to this level.
			if inner := derefType(f.Type); inner != nil && inner.Kind() == reflect.Struct {
				if got := structField(inner, name); got != nil {
					return got
				}
			}
			continue
		}
		want := tagName
		if want == "" {
			want = strings.ToLower(f.Name)
		}
		if want == name {
			return describeType(f.Type)
		}
	}
	return nil
}

// derefType strips pointer/interface layers.
func derefType(t reflect.Type) reflect.Type {
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface) {
		t = t.Elem()
	}
	return t
}

// newElem returns an addressable value that describes what may appear under t
// (a struct's fields, a map's free-form keys, a sequence's elements) so the
// recursion can keep reflecting on it.
func newElem(t reflect.Type) interface{} {
	if t == nil {
		return nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		return newElem(t.Elem())
	case reflect.Interface:
		// any / interface{} exactly like yaml.v3's: nothing is known, so
		// nothing can be "unknown".
		return map[string]interface{}{}
	case reflect.Struct, reflect.Map, reflect.Slice:
		return reflect.New(t).Interface()
	default:
		return struct{}{}
	}
}

// describeType is newElem for a declared field: it keeps the container kind
// (so a map field keeps being treated as free-form) and never returns nil for a
// declared key.
func describeType(t reflect.Type) interface{} {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return struct{}{} // interface field: nothing further is known
	}
	return newElem(t)
}

// warnUnknownKeys prints one line per unknown key. Non-fatal on purpose: a
// config that already runs must not stop running because of a warning, but a key
// that does nothing has to be said out loud.
func warnUnknownKeys(data []byte) {
	keys, err := unknownKeys(data)
	if err != nil || len(keys) == 0 {
		return
	}
	for _, k := range keys {
		fmt.Fprintf(os.Stderr, "\u26a0 %s: 未知配置键（第 %d 行；本程序不读它，拼错了就等于没写）\n", k.Path, k.Line)
	}
}
