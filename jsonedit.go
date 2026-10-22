package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// jnode is a JSON value that keeps its object keys in the order written, so
// an edit can rewrite a file without reshuffling it.
type jnode struct {
	keys  []string // object keys, in order; nil for other kinds
	vals  []*jnode // object values or array items
	raw   string   // a scalar's JSON text
	isObj bool
	isArr bool
}

func parseJSON(s string) (*jnode, error) {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	n, err := decodeNode(d)
	if err != nil {
		return nil, err
	}
	if d.More() {
		return nil, fmt.Errorf("trailing data")
	}
	return n, nil
}

func decodeNode(d *json.Decoder) (*jnode, error) {
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		n := &jnode{isObj: true, keys: []string{}}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return nil, err
			}
			v, err := decodeNode(d)
			if err != nil {
				return nil, err
			}
			n.keys, n.vals = append(n.keys, k.(string)), append(n.vals, v)
		}
		_, err = d.Token()
		return n, err
	case json.Delim('['):
		n := &jnode{isArr: true}
		for d.More() {
			v, err := decodeNode(d)
			if err != nil {
				return nil, err
			}
			n.vals = append(n.vals, v)
		}
		_, err = d.Token()
		return n, err
	}
	raw, _ := json.Marshal(tok)
	return &jnode{raw: string(raw)}, nil
}

func (n *jnode) get(k string) *jnode {
	if i := slices.Index(n.keys, k); i >= 0 {
		return n.vals[i]
	}
	return nil
}

func (n *jnode) equal(o *jnode) bool {
	if n.isObj != o.isObj || n.isArr != o.isArr || n.raw != o.raw || len(n.vals) != len(o.vals) {
		return false
	}
	for i, v := range n.vals {
		if n.isObj {
			ov := o.get(n.keys[i])
			if ov == nil || !v.equal(ov) {
				return false
			}
		} else if !v.equal(o.vals[i]) {
			return false
		}
	}
	return true
}

// contains reports whether every key of part is in n with an equal value,
// looking inside nested objects, so a quoted fragment can be found in the file.
func (n *jnode) contains(part *jnode) bool {
	if !n.isObj || !part.isObj {
		return n.equal(part)
	}
	for i, k := range part.keys {
		v := n.get(k)
		if v == nil || !v.contains(part.vals[i]) {
			return false
		}
	}
	return true
}

// patch applies the difference between old and new to n: keys old has and new
// lacks are removed, changed values are replaced, and new keys are appended.
func (n *jnode) patch(old, new *jnode) {
	for i, k := range old.keys {
		if new.get(k) == nil {
			j := slices.Index(n.keys, k)
			n.keys, n.vals = slices.Delete(n.keys, j, j+1), slices.Delete(n.vals, j, j+1)
		} else if ov, nv, fv := old.vals[i], new.get(k), n.get(k); ov.isObj && nv.isObj && fv.isObj {
			fv.patch(ov, nv)
		} else if !ov.equal(nv) {
			n.vals[slices.Index(n.keys, k)] = nv
		}
	}
	for i, k := range new.keys {
		if old.get(k) == nil {
			if j := slices.Index(n.keys, k); j >= 0 {
				n.vals[j] = new.vals[i]
			} else {
				n.keys, n.vals = append(n.keys, k), append(n.vals, new.vals[i])
			}
		}
	}
}

// objects lists n and every object nested in it.
func (n *jnode) objects() []*jnode {
	var all []*jnode
	if n.isObj {
		all = append(all, n)
	}
	for _, v := range n.vals {
		all = append(all, v.objects()...)
	}
	return all
}

func (n *jnode) write(b *bytes.Buffer) {
	switch {
	case n.isObj:
		b.WriteByte('{')
		for i, k := range n.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			key, _ := json.Marshal(k)
			b.Write(key)
			b.WriteByte(':')
			n.vals[i].write(b)
		}
		b.WriteByte('}')
	case n.isArr:
		b.WriteByte('[')
		for i, v := range n.vals {
			if i > 0 {
				b.WriteByte(',')
			}
			v.write(b)
		}
		b.WriteByte(']')
	default:
		b.WriteString(n.raw)
	}
}

// parseFragment parses a quoted JSON object, closing any braces the model left open.
func parseFragment(s string) *jnode {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ","))
	for i := 0; i < 3; i++ {
		if n, err := parseJSON(s); err == nil {
			if n.isObj {
				return n
			}
			return nil
		}
		if strings.Count(s, "{") <= strings.Count(s, "}") {
			break
		}
		s += "}"
	}
	return nil
}

// patchJSON handles an edit to a JSON file whose old_text is a fragment of it
// quoted in another layout, e.g. a pretty-printed object written on one line.
// The object old_text describes must occur once in the file; the change from
// old_text to new_text is made there and the file is rewritten in its own
// indentation with its key order kept.
func patchJSON(src, old, new string) (string, bool) {
	file, err := parseJSON(src)
	o, nw := parseFragment(old), parseFragment(new)
	if err != nil || o == nil || nw == nil || len(o.keys) == 0 {
		return "", false
	}
	var at *jnode
	for _, obj := range file.objects() {
		if obj.contains(o) {
			if at != nil {
				return "", false
			}
			at = obj
		}
	}
	if at == nil {
		return "", false
	}
	at.patch(o, nw)
	var compact, out bytes.Buffer
	file.write(&compact)
	indent := "  "
	if lines := strings.Split(src, "\n"); len(lines) > 1 && indentOf(lines[1]) != "" {
		indent = indentOf(lines[1])
	}
	json.Indent(&out, compact.Bytes(), "", indent)
	return out.String(), true
}
