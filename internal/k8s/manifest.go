// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/neo4j/cli/internal/clierr"
	"go.yaml.in/yaml/v3"
)

const (
	// DefaultNeo4jVersion matches the version in the operator v1.0.0 examples.
	DefaultNeo4jVersion = "2026.05.0"
	// DefaultStorageSize is the data volume size used by the operator examples.
	DefaultStorageSize = "10Gi"
	// ManagedByLabel identifies resources that the CLI created.
	ManagedByLabel = "app.kubernetes.io/managed-by"
	// ManagedByValue is the label value that instance list selects.
	ManagedByValue = "neo4j-cli"

	defaultMemoryLimit = "4Gi"
	memoryHeadroom     = uint64(1 << 30)
	maxStorageSize     = uint64(16 << 40)
)

var dnsLabelPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateResourceName makes sure that a Kubernetes resource or namespace name
// is a valid RFC 1123 DNS label. This rejects empty names, uppercase letters
// and leading hyphens before any child process runs.
func ValidateResourceName(label, name string) error {
	if len(name) == 0 || len(name) > 63 || !dnsLabelPattern.MatchString(name) {
		return clierr.NewUsageError(
			"invalid %s %q: must be 1 to 63 lowercase letters, digits or hyphens, and start and end with a letter or digit",
			label, name)
	}
	return nil
}

// Neo4jManifestOptions describes the dedicated instance create flags.
// PrepareNeo4jManifestFromFile handles --from-file instead. It keeps unknown
// fields and sets the resource name, namespace and management label.
type Neo4jManifestOptions struct {
	Name                     string
	Namespace                string
	Mode                     string
	Primaries                int
	PrimariesSet             bool
	Version                  string
	StorageSize              string
	StorageClass             string
	Edition                  string
	AcceptLicense            string
	InsecureAdminConnection  bool
	ServiceType              string
	LoadBalancerSourceRanges []string
	Plugins                  []string
	Memory                   string
	CPU                      string
	Heap                     string
	PageCache                string
	Config                   []string
}

type quotedString string

func (s quotedString) MarshalYAML() (any, error) {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: string(s),
		Style: yaml.DoubleQuotedStyle,
	}, nil
}

type neo4jManifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   manifestMetadata `yaml:"metadata"`
	Spec       manifestSpec     `yaml:"spec"`
}

type manifestMetadata struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

type manifestSpec struct {
	Edition      string            `yaml:"edition"`
	Version      quotedString      `yaml:"version"`
	License      *licenseSpec      `yaml:"license,omitempty"`
	Topology     topologySpec      `yaml:"topology"`
	Storage      *storageSpec      `yaml:"storage,omitempty"`
	Auth         *authSpec         `yaml:"auth,omitempty"`
	Config       *configSpec       `yaml:"config,omitempty"`
	Resources    *resourcesSpec    `yaml:"resources,omitempty"`
	Connectivity *connectivitySpec `yaml:"connectivity,omitempty"`
	Trust        *trustSpec        `yaml:"trust,omitempty"`
	Plugins      []string          `yaml:"plugins,omitempty"`
}

type licenseSpec struct {
	Accept quotedString `yaml:"accept"`
}

type topologySpec struct {
	Mode      string    `yaml:"mode"`
	Primaries *poolSpec `yaml:"primaries,omitempty"`
}

type poolSpec struct {
	Members int `yaml:"members"`
}

type storageSpec struct {
	Volumes volumesSpec `yaml:"volumes"`
}

type volumesSpec struct {
	Data dataVolumeSpec `yaml:"data"`
}

type dataVolumeSpec struct {
	Mode    string            `yaml:"mode"`
	Dynamic dynamicVolumeSpec `yaml:"dynamic"`
}

type dynamicVolumeSpec struct {
	Size             quotedString `yaml:"size"`
	StorageClassName quotedString `yaml:"storageClassName,omitempty"`
}

type authSpec struct {
	GeneratePassword bool `yaml:"generatePassword"`
}

type configSpec struct {
	Neo4j map[string]quotedString `yaml:"neo4j,omitempty"`
}

type resourcesSpec struct {
	Requests resourceList `yaml:"requests"`
	Limits   resourceList `yaml:"limits"`
}

type resourceList struct {
	CPU    quotedString `yaml:"cpu,omitempty"`
	Memory quotedString `yaml:"memory,omitempty"`
}

type connectivitySpec struct {
	Service serviceSpec `yaml:"service"`
}

type serviceSpec struct {
	Type                     string   `yaml:"type"`
	LoadBalancerSourceRanges []string `yaml:"loadBalancerSourceRanges,omitempty"`
}

type trustSpec struct {
	Enabled                 bool           `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	InsecureAdminConnection bool           `yaml:"insecureAdminConnection,omitempty" json:"insecureAdminConnection,omitempty"`
	Certificates            map[string]any `yaml:"certificates,omitempty" json:"certificates,omitempty"`
	CertManager             map[string]any `yaml:"certManager,omitempty" json:"certManager,omitempty"`
}

// hasBoltTLS reports whether the trust block secures the Bolt connector. It
// follows the operator's admission rule: TLS must be enabled and a `bolt`
// policy must be listed under certificates. A certManager block only issues
// the listed policies, so on its own it does not secure Bolt.
func (t *trustSpec) hasBoltTLS() bool {
	return t != nil && t.Enabled && t.Certificates["bolt"] != nil
}

// BuildNeo4jManifest makes sure that the dedicated flags are valid and renders
// one Neo4j custom resource. All configuration and quantity values are YAML
// strings.
func BuildNeo4jManifest(opts Neo4jManifestOptions) ([]byte, error) {
	mode, err := canonicalMode(opts.Mode)
	if err != nil {
		return nil, err
	}
	if opts.StorageSize == "" {
		return nil, clierr.NewUsageError("--storage-size must not be empty")
	}
	edition := strings.ToLower(opts.Edition)

	manifest := neo4jManifest{
		APIVersion: "neo4j.com/v1",
		Kind:       "Neo4j",
		Metadata: manifestMetadata{
			Name:      opts.Name,
			Namespace: opts.Namespace,
			Labels:    map[string]string{ManagedByLabel: ManagedByValue},
		},
		Spec: manifestSpec{
			Edition:  edition,
			Version:  quotedString(opts.Version),
			Topology: topologySpec{Mode: mode},
			Storage: &storageSpec{Volumes: volumesSpec{Data: dataVolumeSpec{
				Mode: "Dynamic",
				Dynamic: dynamicVolumeSpec{
					Size:             quotedString(opts.StorageSize),
					StorageClassName: quotedString(opts.StorageClass),
				},
			}}},
			Auth: &authSpec{GeneratePassword: true},
		},
	}
	if edition == "enterprise" {
		manifest.Spec.License = &licenseSpec{Accept: quotedString(strings.ToLower(opts.AcceptLicense))}
	}
	if mode == "Cluster" || opts.PrimariesSet {
		manifest.Spec.Topology.Primaries = &poolSpec{Members: opts.Primaries}
	}
	if mode == "Cluster" || opts.InsecureAdminConnection {
		manifest.Spec.Trust = &trustSpec{InsecureAdminConnection: opts.InsecureAdminConnection}
	}
	if opts.ServiceType != "" {
		serviceType, err := canonicalServiceType(opts.ServiceType)
		if err != nil {
			return nil, err
		}
		manifest.Spec.Connectivity = &connectivitySpec{Service: serviceSpec{
			Type:                     serviceType,
			LoadBalancerSourceRanges: opts.LoadBalancerSourceRanges,
		}}
	}
	manifest.Spec.Plugins = dedupeStrings(opts.Plugins)

	config, err := parseConfig(opts.Config)
	if err != nil {
		return nil, err
	}
	if opts.Heap != "" {
		config["server.memory.heap.initial_size"] = quotedString(opts.Heap)
		config["server.memory.heap.max_size"] = quotedString(opts.Heap)
	}
	if opts.PageCache != "" {
		config["server.memory.pagecache.size"] = quotedString(opts.PageCache)
	}
	if len(config) > 0 {
		manifest.Spec.Config = &configSpec{Neo4j: config}
	}
	if opts.Memory != "" || opts.CPU != "" {
		resources := resourceList{CPU: quotedString(opts.CPU), Memory: quotedString(opts.Memory)}
		manifest.Spec.Resources = &resourcesSpec{Requests: resources, Limits: resources}
	}
	if err := validateNeo4jManifest(manifest); err != nil {
		return nil, err
	}
	return yaml.Marshal(manifest)
}

// ManifestWarnings returns warnings for valid manifest choices that carry an
// operational risk or a permanent topology limit.
func ManifestWarnings(data []byte) []string {
	var manifest neo4jManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil
	}
	if manifest.Spec.Topology.Mode != "Cluster" {
		return nil
	}
	var warnings []string
	if manifest.Spec.Topology.Primaries != nil && manifest.Spec.Topology.Primaries.Members == 1 {
		warnings = append(warnings, "a cluster created with one primary cannot grow its primary pool later.")
	}
	if manifest.Spec.Trust != nil && manifest.Spec.Trust.InsecureAdminConnection {
		warnings = append(warnings, "the operator will send its admin password over an unencrypted pod-network connection.")
	}
	return warnings
}

// PrepareNeo4jManifestFromFile makes sure that a user-supplied Neo4j resource
// is valid. It sets the name, namespace and management label, and keeps all
// other fields unchanged.
func PrepareNeo4jManifestFromFile(data []byte, name, namespace string) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, clierr.NewUsageError("--from-file must contain one Neo4j object")
		}
		return nil, clierr.NewUsageError("parse --from-file: %s", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, clierr.NewUsageError("--from-file must contain exactly one YAML document")
		}
		return nil, clierr.NewUsageError("parse --from-file: %s", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, clierr.NewUsageError("--from-file must contain one Neo4j object")
	}

	root := document.Content[0]
	metadata, err := ensureMapping(root, "metadata")
	if err != nil {
		return nil, clierr.NewUsageError("--from-file metadata must be a mapping")
	}
	setMappingScalar(metadata, "name", name)
	if namespace != "" {
		setMappingScalar(metadata, "namespace", namespace)
	} else {
		deleteMappingKey(metadata, "namespace")
	}
	labels, err := ensureMapping(metadata, "labels")
	if err != nil {
		return nil, clierr.NewUsageError("--from-file metadata.labels must be a mapping")
	}
	setMappingScalar(labels, ManagedByLabel, ManagedByValue)

	var manifest neo4jManifest
	if err := root.Decode(&manifest); err != nil {
		return nil, clierr.NewUsageError("parse --from-file: %s", err)
	}
	if manifest.APIVersion != "neo4j.com/v1" || manifest.Kind != "Neo4j" {
		return nil, clierr.NewUsageError("--from-file must contain apiVersion neo4j.com/v1 and kind Neo4j")
	}
	if err := validateNeo4jManifest(manifest); err != nil {
		return nil, err
	}
	return yaml.Marshal(root)
}

func validateNeo4jManifest(manifest neo4jManifest) error {
	if err := ValidateResourceName("Neo4j name", manifest.Metadata.Name); err != nil {
		return err
	}
	if manifest.Spec.Edition != "enterprise" && manifest.Spec.Edition != "community" {
		return clierr.NewUsageError("spec.edition (--edition) must be enterprise or community")
	}
	if manifest.Spec.Version == "" {
		return clierr.NewUsageError("spec.version (--version) must not be empty")
	}
	if manifest.Spec.Edition == "enterprise" {
		if manifest.Spec.License == nil || (manifest.Spec.License.Accept != "yes" && manifest.Spec.License.Accept != "eval") {
			return clierr.NewUsageError("Enterprise edition requires licence acceptance yes or eval. Use --accept-license or spec.license.accept in --from-file")
		}
	}
	mode := manifest.Spec.Topology.Mode
	if mode != "Standalone" && mode != "Cluster" {
		return clierr.NewUsageError("spec.topology.mode must be Standalone or Cluster")
	}
	if manifest.Spec.Edition == "community" && mode == "Cluster" {
		return clierr.NewUsageError("community edition supports only standalone mode")
	}
	if mode == "Standalone" && manifest.Spec.Topology.Primaries != nil {
		return clierr.NewUsageError("primaries (--primaries) are valid only in cluster mode")
	}
	if mode == "Standalone" && manifest.Spec.Trust != nil && manifest.Spec.Trust.InsecureAdminConnection {
		return clierr.NewUsageError("insecureAdminConnection (--insecure-admin-connection) is valid only in cluster mode")
	}
	if mode == "Cluster" {
		if manifest.Spec.Topology.Primaries == nil {
			return clierr.NewUsageError("cluster mode requires primaries")
		}
		members := manifest.Spec.Topology.Primaries.Members
		if members < 1 || members > 15 || members%2 == 0 {
			return clierr.NewUsageError("the number of primaries must be odd, from 1 to 15")
		}
		trust := manifest.Spec.Trust
		if trust == nil || (!trust.InsecureAdminConnection && !trust.hasBoltTLS()) {
			return clierr.NewUsageError("cluster mode requires --insecure-admin-connection, or Bolt TLS in a --from-file trust block")
		}
		if len(manifest.Spec.Plugins) > 0 {
			return clierr.NewUsageError("plugins are supported only in standalone mode. Set pool plugins in a --from-file manifest")
		}
	}
	for _, plugin := range manifest.Spec.Plugins {
		switch plugin {
		case "apoc", "gds", "bloom":
		default:
			return clierr.NewUsageError("plugin %q is not one of apoc, gds or bloom", plugin)
		}
	}
	if manifest.Spec.Connectivity != nil {
		service := manifest.Spec.Connectivity.Service
		if service.Type != "" && service.Type != "ClusterIP" && service.Type != "LoadBalancer" && service.Type != "NodePort" {
			return clierr.NewUsageError("Service type must be ClusterIP, LoadBalancer or NodePort")
		}
		if service.Type == "LoadBalancer" && len(service.LoadBalancerSourceRanges) == 0 {
			return clierr.NewUsageError("a LoadBalancer Service requires at least one --load-balancer-source-range")
		}
		if service.Type != "LoadBalancer" && len(service.LoadBalancerSourceRanges) > 0 {
			return clierr.NewUsageError("--load-balancer-source-range requires --service-type LoadBalancer")
		}
	}
	if manifest.Spec.Storage != nil {
		size := manifest.Spec.Storage.Volumes.Data.Dynamic.Size
		if size != "" {
			bytes, err := parseKubeQuantity(string(size))
			if err != nil {
				return clierr.NewUsageError("invalid storage size %q: %s", size, err)
			}
			if bytes > maxStorageSize {
				return clierr.NewUsageError("storage size %q exceeds the 16Ti operator limit", size)
			}
		}
	}
	var config map[string]quotedString
	if manifest.Spec.Config != nil {
		config = manifest.Spec.Config.Neo4j
	}
	for key := range config {
		if err := checkOperatorOwned(key); err != nil {
			return err
		}
	}
	return validateMemory(config, manifest.Spec.Resources)
}

// checkOperatorOwned rejects a neo4j.conf key that the operator sets itself.
// Otherwise the operator will refuse the resource at admission or overwrite the
// value.
func checkOperatorOwned(key string) error {
	if replacement, owned := operatorOwnedConfig[key]; owned {
		return clierr.NewUsageError("configuration key %q is owned by the operator. Use %s", key, replacement)
	}
	if strings.HasPrefix(key, "dbms.ssl.policy.") {
		return clierr.NewUsageError("configuration key %q is owned by the operator. Use spec.trust in --from-file", key)
	}
	return nil
}

// validateMemory makes sure that heap, page cache and 1Gi of headroom fit
// inside the container memory limit. Otherwise Kubernetes stops the pod when
// memory runs out.
func validateMemory(config map[string]quotedString, resources *resourcesSpec) error {
	limit := quotedString(defaultMemoryLimit)
	if resources != nil && resources.Limits.Memory != "" {
		limit = resources.Limits.Memory
	}
	limitBytes, err := parseKubeQuantity(string(limit))
	if err != nil {
		return clierr.NewUsageError("invalid memory limit %q: %s", limit, err)
	}
	heapInitial, err := optionalMemory(config["server.memory.heap.initial_size"], "heap initial size")
	if err != nil {
		return err
	}
	heapMax, err := optionalMemory(config["server.memory.heap.max_size"], "heap maximum size")
	if err != nil {
		return err
	}
	if heapInitial > heapMax && heapMax > 0 {
		return clierr.NewUsageError("heap initial size must not exceed heap maximum size")
	}
	heap := max(heapInitial, heapMax)
	pageCache, err := optionalMemory(config["server.memory.pagecache.size"], "page cache")
	if err != nil {
		return err
	}
	if heap == 0 && pageCache == 0 {
		return nil
	}
	// scaleBytes caps each value below 2^62, so the sum cannot overflow.
	if heap+pageCache+memoryHeadroom > limitBytes {
		return clierr.NewUsageError(
			"heap and page cache plus 1Gi of JVM and operating-system headroom exceed the %s memory limit. Raise --memory or reduce --heap and --pagecache",
			limit)
	}
	return nil
}

func optionalMemory(value quotedString, name string) (uint64, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := parseNeo4jSize(string(value))
	if err != nil {
		return 0, clierr.NewUsageError("invalid %s %q: %s", name, value, err)
	}
	return parsed, nil
}

// Kubernetes quantities use decimal suffixes (k, M, G, …) and binary ones
// (Ki, Mi, Gi, …). A lowercase m means milli, so 512m is half a byte, not
// 512 megabytes.
var (
	kubeQuantityPattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE]([+-]?[0-9]+))?([a-zA-Z]*)$`)
	kubeQuantityUnits   = map[string]float64{
		"": 1, "m": 1e-3,
		"k": 1e3, "M": 1e6, "G": 1e9, "T": 1e12, "P": 1e15, "E": 1e18,
		"Ki": 1 << 10, "Mi": 1 << 20, "Gi": 1 << 30, "Ti": 1 << 40, "Pi": 1 << 50, "Ei": 1 << 60,
	}
)

// Neo4j byte sizes are always binary, so 2g is 2GiB. The suffixes match the
// case-sensitive Neo4j parser.
var (
	neo4jSizePattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]*)?|\.[0-9]+)([a-zA-Z]*)$`)
	neo4jSizeUnits   = map[string]float64{
		"": 1, "B": 1,
		"k": 1 << 10, "K": 1 << 10, "kb": 1 << 10, "kB": 1 << 10, "KB": 1 << 10, "KiB": 1 << 10,
		"m": 1 << 20, "M": 1 << 20, "mb": 1 << 20, "mB": 1 << 20, "MB": 1 << 20, "MiB": 1 << 20,
		"g": 1 << 30, "G": 1 << 30, "gb": 1 << 30, "gB": 1 << 30, "GB": 1 << 30, "GiB": 1 << 30,
		"TB": 1 << 40, "TiB": 1 << 40,
	}
)

// parseKubeQuantity returns the number of bytes in a Kubernetes quantity
// such as 4Gi or 500M.
func parseKubeQuantity(value string) (uint64, error) {
	matches := kubeQuantityPattern.FindStringSubmatch(value)
	if matches == nil || (matches[2] != "" && matches[3] != "") {
		return 0, fmt.Errorf("use a positive number with an optional unit such as 512Mi or 4Gi")
	}
	number, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, err
	}
	if matches[2] != "" {
		exponent, err := strconv.Atoi(matches[2])
		if err != nil {
			return 0, err
		}
		number *= math.Pow10(exponent)
	}
	multiplier, ok := kubeQuantityUnits[matches[3]]
	if !ok {
		return 0, fmt.Errorf("unsupported unit %q. Use a Kubernetes unit such as Mi or Gi", matches[3])
	}
	return scaleBytes(number * multiplier)
}

// parseNeo4jSize returns the number of bytes in a Neo4j size value such as
// 2g or 512m.
func parseNeo4jSize(value string) (uint64, error) {
	matches := neo4jSizePattern.FindStringSubmatch(value)
	if matches == nil {
		return 0, fmt.Errorf("use a positive number with an optional unit such as 512m or 2g")
	}
	number, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, err
	}
	multiplier, ok := neo4jSizeUnits[matches[2]]
	if !ok {
		return 0, fmt.Errorf("unsupported unit %q. Use a Neo4j unit such as m or g", matches[2])
	}
	return scaleBytes(number * multiplier)
}

func scaleBytes(bytes float64) (uint64, error) {
	if math.IsNaN(bytes) || bytes < 1 {
		return 0, fmt.Errorf("the size must be at least one byte")
	}
	if math.IsInf(bytes, 1) || bytes >= 1<<62 {
		return 0, fmt.Errorf("the size is too large")
	}
	return uint64(math.Round(bytes)), nil
}

var operatorOwnedConfig = map[string]string{
	"server.bolt.listen_address":                          "spec.connectivity.listeners.bolt",
	"server.bolt.enabled":                                 "spec.connectivity.listeners.bolt",
	"server.bolt.advertised_address":                      "the advertised addresses from the operator",
	"server.bolt.tls_level":                               "spec.trust in --from-file",
	"server.http.listen_address":                          "spec.connectivity.listeners.http",
	"server.http.enabled":                                 "spec.connectivity.listeners.http",
	"server.https.listen_address":                         "spec.connectivity.listeners.https",
	"server.https.enabled":                                "spec.connectivity.listeners.https",
	"server.backup.listen_address":                        "spec.connectivity.listeners.backup",
	"server.backup.enabled":                               "spec.connectivity.listeners.backup",
	"server.jvm.additional":                               "spec.config.jvm.additionalArguments in --from-file",
	"server.metrics.prometheus.enabled":                   "spec.features.monitoring.prometheus in --from-file",
	"server.metrics.prometheus.endpoint":                  "spec.features.monitoring.prometheus in --from-file",
	"dbms.cluster.minimum_initial_system_primaries_count": "spec.topology.minimumMembers in --from-file",
	"initial.dbms.default_primaries_count":                "spec.topology.defaultPrimariesCount in --from-file",
	"initial.dbms.default_secondaries_count":              "spec.topology.secondaries in --from-file",
	"dbms.cluster.discovery.resolver_type":                "cluster discovery in the operator",
	"dbms.kubernetes.discovery.service_port_name":         "cluster discovery in the operator",
	"dbms.kubernetes.label_selector":                      "cluster discovery in the operator",
	"dbms.cluster.raft.binding_timeout":                   "cluster discovery in the operator",
	"dbms.routing.enabled":                                "routing configuration in the operator",
	"dbms.routing.default_router":                         "routing configuration in the operator",
	"dbms.routing.client_side.enforce_for_domains":        "spec.connectivity.clusterDomain in --from-file",
	"server.cluster.advertised_address":                   "the advertised addresses from the operator",
	"server.cluster.raft.advertised_address":              "the advertised addresses from the operator",
	"server.routing.advertised_address":                   "the advertised addresses from the operator",
	"server.cluster.system_database_mode":                 "spec.topology.secondaries in --from-file",
	"initial.server.mode_constraint":                      "spec.topology.secondaries in --from-file",
	"server.logs.config":                                  "spec.logging in --from-file",
	"server.logs.user.config":                             "spec.logging in --from-file",
}

// dedicatedFlagConfig lists the neo4j.conf keys that have their own flag, so
// that each value has one source.
var dedicatedFlagConfig = map[string]string{
	"server.memory.heap.initial_size": "--heap",
	"server.memory.heap.max_size":     "--heap",
	"server.memory.pagecache.size":    "--pagecache",
}

func parseConfig(entries []string) (map[string]quotedString, error) {
	config := map[string]quotedString{}
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			return nil, clierr.NewUsageError("--config must use key=value, got %q", entry)
		}
		if _, exists := config[key]; exists {
			return nil, clierr.NewUsageError("--config key %q is repeated", key)
		}
		if flag, ok := dedicatedFlagConfig[key]; ok {
			return nil, clierr.NewUsageError("set %q with %s, not --config", key, flag)
		}
		config[key] = quotedString(value)
	}
	return config, nil
}

func canonicalMode(mode string) (string, error) {
	switch strings.ToLower(mode) {
	case "standalone":
		return "Standalone", nil
	case "cluster":
		return "Cluster", nil
	default:
		return "", clierr.NewUsageError("--mode must be standalone or cluster")
	}
}

func canonicalServiceType(serviceType string) (string, error) {
	switch strings.ToLower(serviceType) {
	case "clusterip":
		return "ClusterIP", nil
	case "loadbalancer":
		return "LoadBalancer", nil
	case "nodeport":
		return "NodePort", nil
	default:
		return "", clierr.NewUsageError("--service-type must be ClusterIP, LoadBalancer or NodePort")
	}
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func ensureMapping(parent *yaml.Node, key string) (*yaml.Node, error) {
	if existing := mappingValue(parent, key); existing != nil {
		if existing.Kind == yaml.ScalarNode && existing.Tag == "!!null" {
			existing.Kind = yaml.MappingNode
			existing.Tag = "!!map"
			existing.Value = ""
			return existing, nil
		}
		if existing.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s is not a mapping", key)
		}
		return existing, nil
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, node)
	return node, nil
}

// setMappingScalar adds or replaces one string in a mapping.
func setMappingScalar(mapping *yaml.Node, key, value string) {
	if existing := mappingValue(mapping, key); existing != nil {
		existing.Kind = yaml.ScalarNode
		existing.Tag = "!!str"
		existing.Value = value
		existing.Content = nil
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

func deleteMappingKey(mapping *yaml.Node, key string) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}
