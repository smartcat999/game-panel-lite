package v1

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ResourceName is the name identifying various resources in a ResourceList.
type ResourceName string

const (
	ResourceCPU           ResourceName = "cpu"
	ResourceMemory        ResourceName = "memory"
	ResourceStorage       ResourceName = "storage"
	ResourceInstances     ResourceName = "instances"
	ResourceBackups       ResourceName = "backups"
	ResourceBackupStorage ResourceName = "backupStorage"

	MaxPortsPerPlacement = 32
	maxQuantityLength    = 128
)

var (
	decimalPattern     = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)$`)
	countPattern       = regexp.MustCompile(`^[0-9]+$`)
	dimensionNames     = [6]string{"cpu", "memory", "storage", "instances", "backups", "backupStorage"}
	instanceDimensions = [3]ResourceName{ResourceCPU, ResourceMemory, ResourceStorage}
	quotaDimensions    = [6]ResourceName{
		ResourceCPU, ResourceMemory, ResourceStorage,
		ResourceInstances, ResourceBackups, ResourceBackupStorage,
	}
)

// Quantity is a Kubernetes-style fixed-point representation of a number with SI or binary suffixes.
type Quantity string

// ResourceList is a set of (resource name, quantity) pairs, matching Kubernetes corev1.ResourceList.
type ResourceList map[ResourceName]Quantity

// ResourceRequirements describes the compute resource requirements for an Instance.
type ResourceRequirements struct {
	Limits   ResourceList `json:"limits,omitempty"`
	Requests ResourceList `json:"requests,omitempty"`
}

// Code is a resource-control protocol outcome code.
type Code string

const (
	AdmissionDenied      Code = "AdmissionDenied"
	IdentityConflict     Code = "IdentityConflict"
	Closed               Code = "Closed"
	RevisionChanged      Code = "RevisionChanged"
	OutcomeUnknown       Code = "OutcomeUnknown"
	TransientUnavailable Code = "TransientUnavailable"
	InvariantViolation   Code = "InvariantViolation"
)

func (c Code) Error() string { return string(c) }

func (c Code) Valid() bool {
	switch c {
	case AdmissionDenied, IdentityConflict, Closed, RevisionChanged,
		OutcomeUnknown, TransientUnavailable, InvariantViolation:
		return true
	}
	return false
}

// Error preserves the resource-control outcome and the underlying diagnostic.
type Error struct {
	Code        Code
	OperationID string
	Detail      string
	Cause       error
}

func (e *Error) Error() string {
	message := string(e.Code)
	if e.OperationID != "" {
		message += " operation=" + e.OperationID
	}
	if e.Detail != "" {
		message += ": " + e.Detail
	}
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}

func (e *Error) Is(target error) bool {
	code, ok := target.(Code)
	return ok && code == e.Code
}

func (e *Error) Unwrap() error { return e.Cause }

// ValidationError rejects malformed resource/port input before a command is accepted.
type ValidationError struct {
	Field  string
	Value  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s %q: %s", e.Field, e.Value, e.Reason)
}

func invalid(field, value, reason string) error {
	return &ValidationError{Field: field, Value: value, Reason: reason}
}

// ValidateIdentifier exposes the frozen identifier validation policy.
func ValidateIdentifier(field, value string) error {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) {
		return invalid(field, value, "expected a non-empty identifier of at most 256 bytes")
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return invalid(field, value, "identifiers cannot contain whitespace or control characters")
		}
	}
	return nil
}

// ParseCPU parses a CPU Quantity into millicores using exact decimal arithmetic.
func ParseCPU(q Quantity) (int64, error) {
	raw := string(q)
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, nil
	}
	multiplier := int64(1000)
	if strings.HasSuffix(s, "m") {
		s = strings.TrimSuffix(s, "m")
		multiplier = 1
	}
	return parseScaled("cpu", raw, s, multiplier, false)
}

// FormatCPU formats millicores as a canonical Kubernetes CPU Quantity.
func FormatCPU(millicores int64) Quantity {
	if millicores%1000 == 0 {
		return Quantity(strconv.FormatInt(millicores/1000, 10))
	}
	return Quantity(fmt.Sprintf("%dm", millicores))
}

// ParseBytes parses a byte Quantity into bytes using exact decimal arithmetic.
func ParseBytes(q Quantity) (int64, error) {
	raw := string(q)
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, nil
	}
	return parseBytesRaw(raw)
}

func parseBytesRaw(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	units := []struct {
		suffix string
		value  int64
	}{
		{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30},
		{"Ti", 1 << 40}, {"Pi", 1 << 50}, {"Ei", 1 << 60},
		{"k", 1000}, {"K", 1000}, {"M", 1000000}, {"G", 1000000000},
		{"T", 1000000000000}, {"P", 1000000000000000}, {"E", 1000000000000000000},
	}
	for _, unit := range units {
		if strings.HasSuffix(s, unit.suffix) {
			return parseScaled("bytes", raw, strings.TrimSuffix(s, unit.suffix), unit.value, false)
		}
	}
	return parseScaled("bytes", raw, s, 1, false)
}

// FormatBytes formats bytes into a canonical binary SI Quantity.
func FormatBytes(b int64) Quantity {
	const (
		ki = 1024
		mi = 1024 * ki
		gi = 1024 * mi
	)
	if b == 0 {
		return "0"
	}
	if b%gi == 0 {
		return Quantity(fmt.Sprintf("%dGi", b/gi))
	}
	if b%mi == 0 {
		return Quantity(fmt.Sprintf("%dMi", b/mi))
	}
	if b%ki == 0 {
		return Quantity(fmt.Sprintf("%dKi", b/ki))
	}
	return Quantity(strconv.FormatInt(b, 10))
}

// ParseCount parses an integer count Quantity.
func ParseCount(q Quantity) (int64, error) {
	raw := string(q)
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, nil
	}
	return parseScaled("count", raw, s, 1, true)
}

func parseScaled(field, raw, decimal string, multiplier int64, integerOnly bool) (int64, error) {
	pattern := decimalPattern
	if integerOnly {
		pattern = countPattern
	}
	if len(raw) > maxQuantityLength || !pattern.MatchString(decimal) {
		return 0, invalid(field, raw, "expected a bounded non-negative decimal quantity")
	}
	rational, ok := new(big.Rat).SetString(decimal)
	if !ok {
		return 0, invalid(field, raw, "invalid decimal quantity")
	}
	numerator := new(big.Int).Mul(rational.Num(), big.NewInt(multiplier))
	whole, remainder := new(big.Int), new(big.Int)
	whole.QuoRem(numerator, rational.Denom(), remainder)
	if remainder.Sign() != 0 {
		whole.Add(whole, big.NewInt(1))
	}
	if !whole.IsInt64() {
		return 0, invalid(field, raw, "exceeds signed 64-bit accounting units")
	}
	return whole.Int64(), nil
}

// Validate checks that all quantities in the ResourceList are valid Kubernetes quantities.
func (rl ResourceList) Validate() error {
	for name, q := range rl {
		switch name {
		case ResourceCPU:
			if _, err := ParseCPU(q); err != nil {
				return err
			}
		case ResourceMemory, ResourceStorage, ResourceBackupStorage:
			if _, err := ParseBytes(q); err != nil {
				return err
			}
		case ResourceInstances, ResourceBackups:
			if _, err := ParseCount(q); err != nil {
				return err
			}
		default:
			if _, err := ParseCount(q); err != nil {
				if _, bErr := ParseBytes(q); bErr != nil {
					return fmt.Errorf("invalid quantity %q for resource %q", q, name)
				}
			}
		}
	}
	return nil
}

func (rl ResourceList) CPUMillicores() int64 {
	if rl == nil {
		return 0
	}
	v, _ := ParseCPU(rl[ResourceCPU])
	return v
}

func (rl ResourceList) MemoryBytes() int64 {
	if rl == nil {
		return 0
	}
	v, _ := ParseBytes(rl[ResourceMemory])
	return v
}

func (rl ResourceList) StorageBytes() int64 {
	if rl == nil {
		return 0
	}
	v, _ := ParseBytes(rl[ResourceStorage])
	return v
}

func (rl ResourceList) InstancesCount() int64 {
	if rl == nil {
		return 0
	}
	v, _ := ParseCount(rl[ResourceInstances])
	return v
}

func (rl ResourceList) BackupsCount() int64 {
	if rl == nil {
		return 0
	}
	v, _ := ParseCount(rl[ResourceBackups])
	return v
}

func (rl ResourceList) BackupStorageBytes() int64 {
	if rl == nil {
		return 0
	}
	v, _ := ParseBytes(rl[ResourceBackupStorage])
	return v
}

func (rr ResourceRequirements) EffectiveRequests() ResourceList {
	out := ResourceList{}
	for k, v := range rr.Limits {
		out[k] = v
	}
	for k, v := range rr.Requests {
		out[k] = v
	}
	return out
}

func (rr ResourceRequirements) Validate() error {
	if err := rr.Requests.Validate(); err != nil {
		return fmt.Errorf("resources.requests: %w", err)
	}
	if err := rr.Limits.Validate(); err != nil {
		return fmt.Errorf("resources.limits: %w", err)
	}
	return nil
}

// Resources uses integer accounting units.
type Resources struct {
	CPUMillicores      int64 `json:"cpuMillicores"`
	MemoryBytes        int64 `json:"memoryBytes"`
	StorageBytes       int64 `json:"storageBytes"`
	InstanceCount      int64 `json:"instanceCount"`
	BackupsCount       int64 `json:"backupsCount"`
	BackupStorageBytes int64 `json:"backupStorageBytes"`
}

func (r Resources) values() [6]int64 {
	return [6]int64{r.CPUMillicores, r.MemoryBytes, r.StorageBytes, r.InstanceCount, r.BackupsCount, r.BackupStorageBytes}
}

func resources(v [6]int64) Resources {
	return Resources{v[0], v[1], v[2], v[3], v[4], v[5]}
}

func (r Resources) Validate() error {
	for i, value := range r.values() {
		if value < 0 {
			return invalid(dimensionNames[i], strconv.FormatInt(value, 10), "must be non-negative")
		}
	}
	return nil
}

func (r Resources) ValidateNode() error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.BackupsCount != 0 || r.BackupStorageBytes != 0 {
		return invalid("node resources", "backup quota", "backup dimensions belong to the quota domain")
	}
	return nil
}

func (r Resources) ValidateInstance() error {
	if err := r.ValidateNode(); err != nil {
		return err
	}
	if r.CPUMillicores == 0 || r.MemoryBytes == 0 || r.StorageBytes == 0 || r.InstanceCount != 1 {
		return invalid("instance budget", "", "requires positive CPU, memory, storage and exactly one instance")
	}
	return nil
}

func (r Resources) IsZero() bool { return r == (Resources{}) }

func (r Resources) Add(other Resources) (Resources, error) {
	if err := r.Validate(); err != nil {
		return Resources{}, err
	}
	if err := other.Validate(); err != nil {
		return Resources{}, err
	}
	a, b := r.values(), other.values()
	for i := range a {
		if b[i] > math.MaxInt64-a[i] {
			return Resources{}, &Error{Code: InvariantViolation, Detail: dimensionNames[i] + " addition overflow"}
		}
		a[i] += b[i]
	}
	return resources(a), nil
}

func (r Resources) Subtract(other Resources) (Resources, error) {
	if err := r.Validate(); err != nil {
		return Resources{}, err
	}
	if err := other.Validate(); err != nil {
		return Resources{}, err
	}
	a, b := r.values(), other.values()
	for i := range a {
		if b[i] > a[i] {
			return Resources{}, &Error{Code: InvariantViolation, Detail: dimensionNames[i] + " subtraction underflow"}
		}
		a[i] -= b[i]
	}
	return resources(a), nil
}

// Limit distinguishes no entitlement (Value=0) from explicit unlimited capacity.
type Limit struct {
	Value     int64 `json:"value"`
	Unlimited bool  `json:"unlimited"`
}

func (l Limit) Validate() error {
	if l.Value < 0 || (l.Unlimited && l.Value != 0) {
		return invalid("limit", strconv.FormatInt(l.Value, 10), "use non-negative finite value or explicit unlimited with value zero")
	}
	return nil
}

type Limits struct {
	CPU           Limit `json:"cpu"`
	Memory        Limit `json:"memory"`
	Storage       Limit `json:"storage"`
	Instances     Limit `json:"instances"`
	Backups       Limit `json:"backups"`
	BackupStorage Limit `json:"backupStorage"`
}

func (l Limits) values() [6]Limit {
	return [6]Limit{l.CPU, l.Memory, l.Storage, l.Instances, l.Backups, l.BackupStorage}
}

func (l Limits) Validate() error {
	for i, value := range l.values() {
		if err := value.Validate(); err != nil {
			return invalid("limits."+dimensionNames[i], strconv.FormatInt(value.Value, 10), err.Error())
		}
	}
	return nil
}

func (l Limits) CheckIncrease(used, increase Resources) error {
	if err := l.Validate(); err != nil {
		return err
	}
	total, err := used.Add(increase)
	if err != nil {
		return err
	}
	values, delta := total.values(), increase.values()
	for i, limit := range l.values() {
		if delta[i] > 0 && !limit.Unlimited && values[i] > limit.Value {
			return &Error{Code: AdmissionDenied, Detail: dimensionNames[i] + " limit exceeded"}
		}
	}
	return nil
}

func GrowthAndShrink(old, target Resources) (grow, shrink Resources, err error) {
	if err = old.Validate(); err != nil {
		return
	}
	if err = target.Validate(); err != nil {
		return
	}
	a, b := old.values(), target.values()
	var positive, negative [6]int64
	for i := range a {
		if b[i] >= a[i] {
			positive[i] = b[i] - a[i]
		} else {
			negative[i] = a[i] - b[i]
		}
	}
	return resources(positive), resources(negative), nil
}

func parseResource(name ResourceName, value Quantity) (int64, error) {
	raw := string(value)
	switch name {
	case ResourceCPU:
		s := strings.TrimSpace(raw)
		mult := int64(1000)
		if strings.HasSuffix(s, "m") {
			s = strings.TrimSuffix(s, "m")
			mult = 1
		}
		return parseScaled("cpu", raw, s, mult, false)
	case ResourceMemory, ResourceStorage, ResourceBackupStorage:
		return parseBytesRaw(raw)
	case ResourceInstances, ResourceBackups:
		return parseScaled("count", raw, strings.TrimSpace(raw), 1, true)
	default:
		return 0, invalid("resource", string(name), "unknown resource dimension")
	}
}

func validateList(list ResourceList, instanceOnly bool) error {
	for name, value := range list {
		if instanceOnly && name != ResourceCPU && name != ResourceMemory && name != ResourceStorage {
			return invalid("instance resources", string(name), "only cpu, memory and storage are accepted")
		}
		if _, err := parseResource(name, value); err != nil {
			return invalid(string(name), string(value), err.Error())
		}
	}
	return nil
}

func NormalizeInstanceResources(requirements ResourceRequirements, defaults ResourceList) (Resources, error) {
	for _, list := range []ResourceList{requirements.Limits, requirements.Requests, defaults} {
		if err := validateList(list, true); err != nil {
			return Resources{}, err
		}
	}
	var values [3]int64
	for i, name := range instanceDimensions {
		limit, hasLimit := requirements.Limits[name]
		request, hasRequest := requirements.Requests[name]
		if hasLimit && hasRequest {
			l, _ := parseResource(name, limit)
			r, _ := parseResource(name, request)
			if r > l {
				return Resources{}, invalid(string(name), string(request), "normalized request exceeds limit")
			}
		}
		selected, present := limit, hasLimit
		if !present {
			selected, present = request, hasRequest
		}
		if !present {
			selected, present = defaults[name]
		}
		if !present {
			return Resources{}, invalid(string(name), "", "explicit finite ceiling or default is required")
		}
		value, _ := parseResource(name, selected)
		if value == 0 {
			return Resources{}, invalid(string(name), string(selected), "instance executable ceiling must be positive")
		}
		values[i] = value
	}
	return Resources{CPUMillicores: values[0], MemoryBytes: values[1], StorageBytes: values[2], InstanceCount: 1}, nil
}

func NormalizeLimits(input ResourceList, unlimited []ResourceName) (Limits, error) {
	if err := validateList(input, false); err != nil {
		return Limits{}, err
	}
	flags := make(map[ResourceName]bool, len(unlimited))
	for _, name := range unlimited {
		if _, err := parseResource(name, "0"); err != nil {
			return Limits{}, err
		}
		if _, present := input[name]; present || flags[name] {
			return Limits{}, invalid("unlimited", string(name), "duplicate or conflicting numeric limit")
		}
		flags[name] = true
	}
	var limits [6]Limit
	for i, name := range quotaDimensions {
		if flags[name] {
			limits[i] = Limit{Unlimited: true}
		} else if value, present := input[name]; present {
			parsed, _ := parseResource(name, value)
			limits[i] = Limit{Value: parsed}
		}
	}
	return Limits{limits[0], limits[1], limits[2], limits[3], limits[4], limits[5]}, nil
}

// FormatLimits renders parsed Limits back into canonical ResourceList and UnlimitedQuota slices.
func FormatLimits(l Limits) (ResourceList, []ResourceName) {
	quotas := make(ResourceList, len(quotaDimensions))
	var unlimited []ResourceName
	values := l.values()
	for i, name := range quotaDimensions {
		v := values[i]
		if v.Unlimited {
			unlimited = append(unlimited, name)
			continue
		}
		switch name {
		case ResourceCPU:
			quotas[name] = FormatCPU(v.Value)
		case ResourceMemory, ResourceStorage, ResourceBackupStorage:
			quotas[name] = FormatBytes(v.Value)
		default:
			quotas[name] = Quantity(strconv.FormatInt(v.Value, 10))
		}
	}
	return quotas, unlimited
}

func NormalizeNodeLimits(capacity, allocatable ResourceList) (Limits, error) {
	input := ResourceList{}
	for _, key := range []ResourceName{ResourceCPU, ResourceMemory, ResourceStorage, ResourceInstances} {
		if value, present := allocatable[key]; present {
			input[key] = value
		} else if value, present := capacity[key]; present {
			input[key] = value
		}
	}
	var unlimited []ResourceName
	if _, present := input[ResourceInstances]; !present {
		unlimited = []ResourceName{ResourceInstances}
	}
	return NormalizeLimits(input, unlimited)
}

func CanonicalResources(r Resources) ResourceList {
	return ResourceList{
		ResourceCPU:           Quantity(strconv.FormatInt(r.CPUMillicores, 10) + "m"),
		ResourceMemory:        Quantity(strconv.FormatInt(r.MemoryBytes, 10)),
		ResourceStorage:       Quantity(strconv.FormatInt(r.StorageBytes, 10)),
		ResourceInstances:     Quantity(strconv.FormatInt(r.InstanceCount, 10)),
		ResourceBackups:       Quantity(strconv.FormatInt(r.BackupsCount, 10)),
		ResourceBackupStorage: Quantity(strconv.FormatInt(r.BackupStorageBytes, 10)),
	}
}

func ParseResources(input ResourceList) (Resources, error) {
	l, err := NormalizeLimits(input, nil)
	if err != nil {
		return Resources{}, err
	}
	return Resources{CPUMillicores: l.CPU.Value, MemoryBytes: l.Memory.Value, StorageBytes: l.Storage.Value, InstanceCount: l.Instances.Value, BackupsCount: l.Backups.Value, BackupStorageBytes: l.BackupStorage.Value}, nil
}

func InstanceRequirements(r Resources) ResourceRequirements {
	all := CanonicalResources(r)
	limits := ResourceList{ResourceCPU: all[ResourceCPU], ResourceMemory: all[ResourceMemory], ResourceStorage: all[ResourceStorage]}
	requests := ResourceList{ResourceCPU: limits[ResourceCPU], ResourceMemory: limits[ResourceMemory], ResourceStorage: limits[ResourceStorage]}
	return ResourceRequirements{Requests: requests, Limits: limits}
}

type PortRequest struct {
	Name          string `json:"name"`
	Protocol      string `json:"protocol"`
	TargetPort    int32  `json:"targetPort"`
	PublishedPort int32  `json:"publishedPort"`
}

func CanonicalPorts(input []PortRequest) ([]PortRequest, error) {
	if len(input) > MaxPortsPerPlacement {
		return nil, invalid("ports", "", "placement exceeds the bounded port set")
	}
	out := slices.Clone(input)
	if out == nil {
		out = []PortRequest{}
	}
	names, fixed, exact := map[string]bool{}, map[string]bool{}, map[PortRequest]bool{}
	for i := range out {
		if err := normalizeAndCheckPort(&out[i], names, fixed, exact); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func normalizeAndCheckPort(p *PortRequest, names, fixed map[string]bool, exact map[PortRequest]bool) error {
	p.Protocol = strings.ToUpper(strings.TrimSpace(p.Protocol))
	if p.Protocol == "" {
		p.Protocol = ProtocolTCP
	}
	if p.Protocol != ProtocolTCP && p.Protocol != ProtocolUDP {
		return invalid("protocol", p.Protocol, "expected TCP or UDP")
	}
	if p.TargetPort < 1 || p.TargetPort > 65535 || p.PublishedPort < 0 || p.PublishedPort > 65535 {
		return invalid("port", p.Name, "target must be 1..65535; published must be 0..65535")
	}
	if p.Name != "" {
		if err := ValidateIdentifier("port name", p.Name); err != nil {
			return err
		}
		if names[p.Name] {
			return invalid("port name", p.Name, "duplicate")
		}
		names[p.Name] = true
	}
	key := p.Protocol + "/" + strconv.FormatInt(int64(p.PublishedPort), 10)
	if exact[*p] || (p.PublishedPort > 0 && fixed[key]) {
		return invalid("port", key, "duplicate request or fixed host-port claim")
	}
	exact[*p] = true
	if p.PublishedPort > 0 {
		fixed[key] = true
	}
	return nil
}
