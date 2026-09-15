package config

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// extendsKey is the registry-entry key that names the entry this one inherits
// from. It is a real ModelConfig field (Extends) so `docvision setup`'s strict
// KnownFields check accepts it; the loader consumes it while merging.
const extendsKey = "extends"

// modelsKey / yamlMapTag / yamlNullTag are the literal names yaml.v3 reports
// for these node kinds; they appear in every error path we build by hand.
const (
	modelsKey   = "models"
	yamlMapTag  = "!!map"
	yamlNullTag = "!!null"
)

// mergeModelExtends resolves every `models.<name>.extends: <other>` reference by
// deep-merging the parent entry into the child AT THE YAML NODE LEVEL, before
// the document is decoded into Config. The node layer is the only place this
// can work: ModelConfig is nearly all value types, so after decoding there is
// no way to tell "key absent" (inherit) from "key written as 0/false/\"\""
// (override).
//
// Semantics of the merge (child = the entry that writes `extends`, parent = the
// entry it names). The merge runs on the ENTRY mapping, key by key over the
// entry's own keys (model / base_url / api_key / price / thinking / …) — it does
// not descend into a value the child wrote itself:
//
//	key absent in child      inherits the parent's value
//	key present in child     the child's value wins wholesale — including a map
//	                         value like request_body (a nested block is never
//	                         half-merged: the child writes the block it wants)
//	                         and including a list (lists are NOT concatenated)
//	key: null                deletes an inherited key ("explicit clear")
//	explicit 0 / false / ""  counts as an override — the criterion is whether
//	                         the key is present, not whether the value is non-zero
//
// Deeper per-key inheritance exists where the runtime already defines it, and
// keeps working underneath: ResolveModel fills models.text's credentials plus
// request_body/thinking into an entry that left them out, ResolveImageEstimate
// fills estimate.* into a partial image_tokens block, and a price block that
// sets no rate at all falls back to models.text's rates. See docs/config.md.
//
// Only the child's OWN keys are written back into the document; every key it
// did not write stays "absent" there, which is what keeps ResolveModel's
// zero-value inheritance (a second, unrelated rule — see docs/config.md)
// working exactly as before for configs that never use extends.
//
// Errors are fatal and name the full chain: unknown parent, self-reference and
// cycles all print `models.<名>.extends` plus the loop.
func mergeModelExtends(root *yaml.Node) error {
	models := mapNodeValue(documentRoot(root), modelsKey)
	if models == nil || models.Kind != yaml.MappingNode {
		return nil
	}

	// extendsOf remembers every declared reference even after the key is
	// consumed from the document, because cycle reports need the raw edges.
	extendsOf := map[string][]string{}
	for i := 0; i+1 < len(models.Content); i += 2 {
		name := models.Content[i].Value
		entry := models.Content[i+1]
		if entry.Kind != yaml.MappingNode {
			continue
		}
		parents, ok, err := extendsRefs(name, entry)
		if err != nil {
			return err
		}
		if ok {
			extendsOf[name] = parents
		}
	}

	// A child can only be merged once its parent is already a complete map, so
	// run passes until nothing changes. Names are visited in sorted order to
	// keep the result independent of map iteration order.
	for pass := 0; pass <= len(extendsOf); pass++ {
		progress := false
		names := make([]string, 0, len(extendsOf))
		for name := range extendsOf {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			parents := extendsOf[name]
			ready, firstMissing := true, ""
			for _, parentName := range parents {
				parent, ok := lookupModel(models, parentName)
				if !ok {
					return fmt.Errorf("models.%s.%s: 引用的基座条目 %q 不存在（先定义 `%s:`，再让别的条目 `%s: %s`）",
						name, extendsKey, parentName, parentName, extendsKey, parentName)
				}
				// The parent still carries its own extends key from an earlier
				// pass? Then it has not been resolved yet — wait for a later pass.
				if hasKey(parent, extendsKey) {
					ready = false
					break
				}
				_ = firstMissing
			}
			if !ready {
				continue
			}
			// T25 多基座：按书写顺序折叠——后面的基座覆盖前面的。折叠在一个
			// **浅拷贝**上进行（applyExtends 会重写 child.Content，直接用父条目
			// 节点会把文档里的基座改掉）：acc = p1；acc = pᵢ 覆盖 acc；最后条目
			// 自己的键覆盖所有基座。
			first, ok0 := lookupModel(models, parents[0])
			if !ok0 {
				// ready 循环已确认存在；防御性兜底。
				return fmt.Errorf("models.%s.%s: 引用的基座条目 %q 不存在", name, extendsKey, parents[0])
			}
			acc := shallowMapCopy(first)
			for _, parentName := range parents[1:] {
				pn, _ := lookupModel(models, parentName)
				tmp := shallowMapCopy(pn)
				if err := applyExtends(tmp, acc); err != nil {
					return fmt.Errorf("models.%s.%s: %w", name, extendsKey, err)
				}
				acc = tmp
			}
			child, _ := lookupModel(models, name)
			if err := applyExtends(child, acc); err != nil {
				return fmt.Errorf("models.%s.%s: %w", name, extendsKey, err)
			}
			delete(extendsOf, name)
			progress = true
		}
		if len(extendsOf) == 0 {
			return nil
		}
		if !progress {
			break
		}
	}

	// Whatever is left is a cycle (or a chain hanging off one).
	cycles := make([]string, 0, len(extendsOf))
	for name := range extendsOf {
		cycles = append(cycles, name)
	}
	sort.Strings(cycles)
	name := cycles[0]
	return fmt.Errorf("models.%s.%s: 循环继承（%s）", name, extendsKey, extendsChain(name, extendsOf))
}

// extendsRef reads one entry's extends key. A present-but-empty value is an
// error rather than a silent no-op, because the entry then looks inherited
// while every key actually comes from the code defaults.
func extendsRef(name string, entry *yaml.Node) (string, bool, error) {
	refs, ok, err := extendsRefs(name, entry)
	if err != nil || !ok {
		return "", ok, err
	}
	return refs[0], true, nil
}

// extendsRefs reads one entry's extends key. T25：除了单个条目名，还接受**列表**
// ——按书写顺序合并、后面的基座覆盖前面的，条目自己的键覆盖所有基座（YAML 的
// 重复键会直接报错，多重继承因此必须写成列表）。A present-but-empty value is an
// error (the key was written, so the user clearly meant something).
func extendsRefs(name string, entry *yaml.Node) ([]string, bool, error) {
	key, value := mapNodeEntry(entry, extendsKey)
	if key == nil {
		return nil, false, nil
	}
	switch value.Kind {
	case yaml.SequenceNode:
		if len(value.Content) == 0 {
			return nil, false, fmt.Errorf("models.%s.%s: 列表不能为空（删掉这个键，或写至少一个条目名）", name, extendsKey)
		}
		out := make([]string, 0, len(value.Content))
		for i, it := range value.Content {
			if it.Kind != yaml.ScalarNode || strings.TrimSpace(it.Value) == "" {
				return nil, false, fmt.Errorf("models.%s.%s: 列表第 %d 项必须是另一个 models 条目的名字（非空字符串）", name, extendsKey, i+1)
			}
			out = append(out, strings.TrimSpace(it.Value))
		}
		return out, true, nil
	default:
		if value.Kind != yaml.ScalarNode || strings.TrimSpace(value.Value) == "" {
			return nil, false, fmt.Errorf("models.%s.%s: 必须写成另一个 models 条目的名字（字符串或条目名列表，不能为空）", name, extendsKey)
		}
		return []string{strings.TrimSpace(value.Value)}, true, nil
	}
}

// shallowMapCopy copies a models entry's mapping shell (sharing key/value
// nodes) so multi-extends folding never mutates the parent entries in place.
func shallowMapCopy(n *yaml.Node) *yaml.Node {
	c := &yaml.Node{Kind: yaml.MappingNode, Tag: yamlMapTag}
	c.Content = append(c.Content, n.Content...)
	return c
}

// applyExtends writes the parent's keys into the child in place, keeping the
// child's mapping order (so the merged document still reads like the entry the
// user wrote): every key the child did not write is appended in the parent's
// order, keys the child wrote keep the child's value, and `key: null` on the
// child side clears an inherited key for good. The child's own extends key is
// consumed here.
func applyExtends(child, parent *yaml.Node) error {
	own := map[string]*yaml.Node{}
	for i := 0; i+1 < len(child.Content); i += 2 {
		own[child.Content[i].Value] = child.Content[i+1]
	}
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: yamlMapTag}
	out.Content = make([]*yaml.Node, 0, len(child.Content)+len(parent.Content))
	for i := 0; i+1 < len(child.Content); i += 2 {
		k, v := child.Content[i], child.Content[i+1]
		if k.Value == extendsKey {
			continue
		}
		if isNullNode(v) {
			// `key: null` = explicit clear: the key stays absent after the merge.
			continue
		}
		out.Content = append(out.Content, k, v)
	}
	for i := 0; i+1 < len(parent.Content); i += 2 {
		k, v := parent.Content[i], parent.Content[i+1]
		if k.Value == extendsKey {
			continue
		}
		if _, overridden := own[k.Value]; overridden {
			continue
		}
		if isNullNode(v) {
			continue // a null in the parent contributes nothing either
		}
		out.Content = append(out.Content, k, v)
	}
	child.Content = out.Content
	return nil
}

// isNullNode reports whether a node is an explicit `null`.
func isNullNode(n *yaml.Node) bool {
	return n != nil && n.Kind == yaml.ScalarNode && n.Tag == yamlNullTag
}

// extendsChain renders the closed walk from name back to itself, e.g.
// "a → b → a". With multi-parent edges it follows the first parent that is
// still unresolved (that is where the cycle lives).
func extendsChain(start string, edges map[string][]string) string {
	parts := []string{start}
	cur := start
	for i := 0; i <= len(edges); i++ {
		parents, ok := edges[cur]
		if !ok {
			break
		}
		next := ""
		for _, p := range parents {
			if _, hanging := edges[p]; hanging {
				next = p
				break
			}
		}
		if next == "" && len(parents) > 0 {
			next = parents[0]
		}
		if next == "" {
			break
		}
		parts = append(parts, next)
		if next == start {
			break
		}
		cur = next
	}
	return strings.Join(parts, " → ")
}

// documentRoot unwraps to the top-level mapping node.
func documentRoot(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil
		}
		return n.Content[0]
	}
	return n
}

// mapNodeValue returns the value node for key inside a mapping node.
func mapNodeValue(n *yaml.Node, key string) *yaml.Node {
	_, v := mapNodeEntry(n, key)
	return v
}

// mapNodeEntry returns the key/value node pair for key inside a mapping node.
func mapNodeEntry(n *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i], n.Content[i+1]
		}
	}
	return nil, nil
}

// lookupModel finds models.<name> in the registry mapping.
func lookupModel(models *yaml.Node, name string) (*yaml.Node, bool) {
	v := mapNodeValue(models, name)
	if v == nil || v.Kind != yaml.MappingNode {
		return nil, false
	}
	return v, true
}

// hasKey reports whether a mapping still carries key.
func hasKey(n *yaml.Node, key string) bool {
	k, _ := mapNodeEntry(n, key)
	return k != nil
}
