// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

// RenameKey copies m and returns a new map where the key from is replaced by
// to. If from is absent the map is returned unchanged (still a copy). When
// preferTo is true and m already holds a native to value, that value is kept and
// from is simply dropped; when false, from's value is moved onto to
// (overwriting any native to).
func RenameKey(m map[string]any, from, to string, preferTo bool) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k == from || k == to {
			continue
		}
		out[k] = v
	}
	if native, ok := m[to]; preferTo && ok {
		out[to] = native
	} else if legacy, ok := m[from]; ok {
		out[to] = legacy
	} else if native, ok := m[to]; ok {
		out[to] = native
	}
	return out
}

// str returns m[key] when it is a string, else "".
func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
