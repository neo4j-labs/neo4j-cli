// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/spf13/afero"
)

func GetTypeDefsFromFlag(cfg *clicfg.Config, typeDefs string, typeDefsFile string) (string, error) {
	typeDefsForBody := ""
	if typeDefs != "" {
		_, err := base64.StdEncoding.DecodeString(typeDefs)
		if err != nil {
			return "", errors.New("provided type definitions are not valid base64")
		}
		// type defs in request body need to be base 64 encoded
		typeDefsForBody = typeDefs
	} else {
		base64EncodedTypeDefs, err := ResolveTypeDefsFileFlagValue(cfg.Aura.Fs(), typeDefsFile)
		if err != nil {
			return "", err
		}

		typeDefsForBody = base64EncodedTypeDefs
	}

	return typeDefsForBody, nil
}

func ResolveTypeDefsFileFlagValue(fs afero.Fs, typeDefsFileFlagValue string) (string, error) {
	data := fileutils.ReadFileSafe(fs, typeDefsFileFlagValue)
	if len(data) == 0 {
		return "", fmt.Errorf("type definitions file '%s' does not exist", typeDefsFileFlagValue)
	}

	base64EncodedTypeDefs := base64.StdEncoding.EncodeToString([]byte(data))

	return base64EncodedTypeDefs, nil
}
