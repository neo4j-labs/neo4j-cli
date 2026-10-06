// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import "slices"

// DataVolumeName is the name of the named volume `docker load` creates for a
// container's /data. It is derived from the container name, so the loader that
// creates it and `docker delete` that offers to remove it cannot disagree.
func DataVolumeName(container string) string {
	return "neo4j-cli-" + container + "-data"
}

// ManagedDataVolume returns the CLI-created data volume of c, if it has one: the
// volume DataVolumeName(c.Name) that is actually mounted into the container. A
// volume the user attached themselves, or a bind mount, is never returned, so
// removing "the container's data volume" can never delete something the CLI did
// not create.
func ManagedDataVolume(c Container) (string, bool) {
	want := DataVolumeName(c.Name)
	if slices.Contains(c.Volumes, want) {
		return want, true
	}
	return "", false
}
