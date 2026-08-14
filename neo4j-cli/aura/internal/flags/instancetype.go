// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import "errors"

// InstanceType is the --type flag for the instance create/deploy/load leaves,
// which POST to the v2beta1 org/project-scoped instances endpoint. That endpoint
// accepts only the v2beta1 tier vocabulary ("free", "professional",
// "business-critical", "virtual-dedicated-cloud"); the older v1 "*-db" names are
// rejected by the API with a validation_error.
//
// The v1 names are still accepted as INPUT aliases (see instanceTypeAliases) so
// existing scripts, agent skills, and docs keep working, but Set normalizes them
// to the v2beta1 name. The value stored in an InstanceType is therefore always
// canonical, which every downstream comparison (free-tier flag rules, request
// body construction, stored-credential database naming) relies on.
//
// Not to be confused with LegacyInstanceType, which is still v1-only.
type InstanceType string

// instanceTypeAliases maps the v1 instance type names onto their v2beta1
// equivalents. AuraDB Enterprise was renamed Virtual Dedicated Cloud;
// "business-critical" kept its name and so needs no alias.
var instanceTypeAliases = map[string]string{
	"free-db":         "free",
	"professional-db": "professional",
	"enterprise-db":   "virtual-dedicated-cloud",
}

// String is used both by fmt.Print and by Cobra in help text
func (e *InstanceType) String() string {
	return string(*e)
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *InstanceType) Set(v string) error {
	if canonical, ok := instanceTypeAliases[v]; ok {
		v = canonical
	}

	switch v {
	case "free", "professional", "business-critical", "virtual-dedicated-cloud":
		*e = InstanceType(v)
		return nil
	case "professional-ds", "enterprise-ds":
		// The v2beta1 API has no AuraDS tier: graph data science runs on a
		// standard instance and is configured separately. Accepting these
		// locally would only defer the failure to the API.
		return errors.New(`AuraDS instance types are no longer offered; create a "professional" or "virtual-dedicated-cloud" instance instead, and configure graph analytics separately`)
	default:
		return errors.New(`must be one of "free", "professional", "business-critical", or "virtual-dedicated-cloud"`)
	}
}

// Type is only used in help text
func (e *InstanceType) Type() string {
	return "type"
}

// LegacyInstanceType is the --type flag for `customer-managed-key create`, which
// still POSTs to the v1 /customer-managed-keys endpoint and therefore still
// speaks the v1 tier vocabulary. Keep it separate from InstanceType: the two
// endpoints disagree on the wire names, so normalizing this flag would send
// v2beta1 names to a v1 endpoint. Fold it into InstanceType only once the
// customer-managed-key commands are migrated to v2beta1.
type LegacyInstanceType string

// String is used both by fmt.Print and by Cobra in help text
func (e *LegacyInstanceType) String() string {
	return string(*e)
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *LegacyInstanceType) Set(v string) error {
	switch v {
	case "free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", "enterprise-ds":
		*e = LegacyInstanceType(v)
		return nil
	default:
		return errors.New(`must be one of "free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", or "enterprise-ds"`)
	}
}

// Type is only used in help text
func (e *LegacyInstanceType) Type() string {
	return "type"
}
