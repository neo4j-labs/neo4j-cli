// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import "context"

// JobSpec describes a one-shot container: it runs a single command to
// completion and is removed (`docker run --rm`). The dataset loader uses one to
// run `neo4j-admin database load`; a future dump or backup command would too.
//
// Unlike ServerSpec it carries no credentials, so Env holds plain, non-secret
// values rendered as `-e NAME=value`. A job must never put a secret there:
// argv is world-readable via /proc/<pid>/cmdline.
//
// There is deliberately no field for an entrypoint override. A job always runs
// through the image's default entrypoint (see DatabaseLoadJob for why that
// matters).
type JobSpec struct {
	Image  string
	Mounts []Mount
	Env    []string // NAME=value, non-secret
	Cmd    []string // the command and its arguments, after the image
}

// Args returns the arguments for `docker run` (everything after `run`), in a
// fixed order: --rm, mounts, environment, image, command.
func (j JobSpec) Args() []string {
	argv := []string{"--rm"}
	for _, m := range j.Mounts {
		argv = append(argv, "-v", m.arg())
	}
	for _, e := range j.Env {
		argv = append(argv, "-e", e)
	}
	argv = append(argv, j.Image)
	return append(argv, j.Cmd...)
}

// RunJob runs the job in the foreground and waits for it to finish.
func RunJob(ctx context.Context, client Client, job JobSpec) error {
	_, err := client.Run(ctx, job.Args())
	return err
}

// DatabaseLoadJob returns the job that loads a dump into a named volume with
// `neo4j-admin database load`. importDir is the host directory holding
// <database>.dump (neo4j-admin requires that file name); it is mounted read-only
// at LoaderImportDir and volume is mounted at /data.
//
// The invariants this encodes, each learned the hard way:
//   - It runs through the image's DEFAULT entrypoint (no --entrypoint override),
//     so docker-entrypoint.sh drops to the neo4j user (exec su-exec neo4j:neo4j
//     "$@") before neo4j-admin runs. The loaded /data/databases/<db> files are
//     then owned by uid 7474, matching the server container. Run as root instead
//     and the server, which drops to neo4j, cannot write the root-owned files,
//     leaving the database offline.
//   - The default entrypoint enforces the enterprise licence gate, so the job
//     accepts it (eval) or neo4j-admin never runs and the database ends up empty.
func DatabaseLoadJob(image, importDir, volume, database string) JobSpec {
	return JobSpec{
		Image: image,
		Mounts: []Mount{
			{Source: importDir, Target: LoaderImportDir, ReadOnly: true},
			{Source: volume, Target: "/data"},
		},
		Env: []string{"NEO4J_ACCEPT_LICENSE_AGREEMENT=eval"},
		Cmd: []string{
			"neo4j-admin", "database", "load", database,
			"--from-path=" + LoaderImportDir,
			"--overwrite-destination=true",
		},
	}
}
