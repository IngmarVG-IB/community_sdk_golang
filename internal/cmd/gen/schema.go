package main

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/bufbuild/protocompile"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Pagination styles detected on request messages.
const (
	pagingNone   = ""
	pagingConfig = "pagination_config" // field `kentik.alerting.types.v202506.PaginationConfig pagination`
	pagingOffset = "offset_limit"      // top-level integer `offset` and `limit` fields
)

const goModule = "github.com/kentik/api-schema-public"

// Schema is the parsed api-schema-public tree.
type Schema struct {
	Ref  string // module version
	APIs []*API
}

// API is a directory under proto/kentik that has versions, e.g. "synthetics" or "alerting/public".
type API struct {
	Path     string
	Versions []*Version // sorted oldest first
}

// Version is one API version with at least one gRPC service.
type Version struct {
	API       *API
	Name      string // e.g. "v202309"
	Stability string // stable, beta, alpha
	Dir       string // e.g. "synthetics/v202309"
	GoImport  string
	Services  []*Service

	// Exposure, decided by classify.
	Exposure   string // exposureStable, exposureExperimental or exposureNone
	IsAlias    bool   // target of the unversioned alias in package kentikapi
	Superseded bool   // pre-release not wrapped because a stable or newer version exists
}

// Exposure kinds.
const (
	exposureStable       = "stable"
	exposureExperimental = "experimental"
	exposureNone         = "none"
)

// Service is one gRPC service.
type Service struct {
	Name        string // e.g. "SyntheticsAdminService"
	FullName    string
	DefaultHost string
	Comment     string
	Methods     []*Method
}

// Method is one RPC.
type Method struct {
	Name, FullMethod     string
	HTTPMethod, HTTPPath string
	ClientStreaming      bool
	ServerStreaming      bool
	Idempotent           bool
	Pagination           string
}

var versionRE = regexp.MustCompile(`^v[0-9]+((alpha|beta)[0-9]+)?$`)

func stabilityOf(version string) string {
	switch {
	case strings.Contains(version, "alpha"):
		return "alpha"
	case strings.Contains(version, "beta"):
		return "beta"
	default:
		return "stable"
	}
}

// LoadSchema compiles every proto under <root>/proto/kentik.
func LoadSchema(ctx context.Context, root, ref string) (*Schema, error) {
	protoRoot := filepath.Join(root, "proto")
	var files []string
	err := filepath.WalkDir(filepath.Join(protoRoot, "kentik"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".proto") {
			rel, err := filepath.Rel(protoRoot, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk protos: %w", err)
	}
	sort.Strings(files)

	vendor := filepath.Join(root, "protovendor", "github.com")
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{ImportPaths: []string{
			protoRoot,
			filepath.Join(vendor, "googleapis", "googleapis"),
			filepath.Join(vendor, "grpc-ecosystem", "grpc-gateway"),
			filepath.Join(vendor, "protocolbuffers", "protobuf", "src"),
		}}),
		SourceInfoMode: protocompile.SourceInfoStandard,
	}
	compiled, err := compiler.Compile(ctx, files...)
	if err != nil {
		return nil, fmt.Errorf("compile protos: %w", err)
	}

	apis := map[string]*API{}
	versions := map[string]*Version{}
	for _, fd := range compiled {
		if fd.Services().Len() == 0 {
			continue
		}
		dir := strings.TrimPrefix(filepath.ToSlash(filepath.Dir(fd.Path())), "kentik/")
		apiPath, ver := filepath.ToSlash(filepath.Dir(dir)), filepath.Base(dir)
		if !versionRE.MatchString(ver) {
			return nil, fmt.Errorf("%s: unexpected version directory %q", fd.Path(), ver)
		}
		// protoc-gen-go runs with paths=source_relative, so the Go package follows the proto directory
		// (not go_package, which differs for audit/public).
		if err := checkGenerated(root, dir); err != nil {
			return nil, err
		}
		a := apis[apiPath]
		if a == nil {
			a = &API{Path: apiPath}
			apis[apiPath] = a
		}
		v := versions[dir]
		if v == nil {
			v = &Version{
				API: a, Name: ver, Stability: stabilityOf(ver), Dir: dir,
				GoImport: goModule + "/gen/go/kentik/" + dir,
			}
			versions[dir] = v
			a.Versions = append(a.Versions, v)
		}
		for i := range fd.Services().Len() {
			v.Services = append(v.Services, newService(fd.Services().Get(i)))
		}
	}

	s := &Schema{Ref: ref}
	for _, a := range apis {
		sort.Slice(a.Versions, func(i, j int) bool { return versionLess(a.Versions[i].Name, a.Versions[j].Name) })
		for _, v := range a.Versions {
			sort.Slice(v.Services, func(i, j int) bool { return v.Services[i].Name < v.Services[j].Name })
		}
		s.APIs = append(s.APIs, a)
	}
	sort.Slice(s.APIs, func(i, j int) bool { return s.APIs[i].Path < s.APIs[j].Path })
	classify(s)
	return s, nil
}

func checkGenerated(root, dir string) error {
	matches, err := filepath.Glob(filepath.Join(root, "gen", "go", "kentik", filepath.FromSlash(dir), "*_grpc.pb.go"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no generated gRPC code in gen/go/kentik/%s", dir)
	}
	return nil
}

// versionLess orders versions by date, and a stable version after pre-releases of the same date.
func versionLess(a, b string) bool {
	da, db := versionDate(a), versionDate(b)
	if da != db {
		// Dates have different precision (v202309 vs v20261005); compare on the first 6 digits, then length.
		pa, pb := da[:min(6, len(da))], db[:min(6, len(db))]
		if pa != pb {
			return pa < pb
		}
		return da < db
	}
	sa, sb := stabilityOf(a), stabilityOf(b)
	if sa != sb {
		return rank(sa) < rank(sb)
	}
	return a < b
}

func versionDate(v string) string {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "ab"); i >= 0 {
		return v[:i]
	}
	return v
}

func rank(stability string) int {
	return map[string]int{"alpha": 0, "beta": 1, "stable": 2}[stability]
}

// classify decides which versions get accessors:
//   - every stable version: package kentikapi;
//   - the newest pre-release of an API with no stable version, or the newest pre-release that adds a service
//     not present in any stable version: package experimental;
//   - everything else is superseded and only reachable through Client.Conn().
//
// The unversioned alias points at the newest stable version whose services are a superset of every older
// stable version's services, so a partial new version (e.g. alerting/public v202611, suppressions only)
// does not hide the full one.
func classify(s *Schema) {
	for _, a := range s.APIs {
		stableServices := map[string]bool{}
		var stable []*Version
		for _, v := range a.Versions {
			v.Exposure = exposureNone
			if v.Stability == "stable" {
				v.Exposure = exposureStable
				stable = append(stable, v)
				for _, svc := range v.Services {
					stableServices[svc.Name] = true
				}
			}
		}
		for i := len(stable) - 1; i >= 0; i-- {
			if coversAll(stable[i], stable[:i]) {
				stable[i].IsAlias = true
				break
			}
		}
		var newestPre *Version
		for i := len(a.Versions) - 1; i >= 0; i-- {
			v := a.Versions[i]
			if v.Stability == "stable" {
				continue
			}
			adds := slices.ContainsFunc(v.Services, func(svc *Service) bool { return !stableServices[svc.Name] })
			if newestPre == nil && (len(stable) == 0 || adds) {
				newestPre = v
				v.Exposure = exposureExperimental
				continue
			}
			v.Superseded = true
		}
	}
}

func coversAll(v *Version, older []*Version) bool {
	have := map[string]bool{}
	for _, s := range v.Services {
		have[s.Name] = true
	}
	for _, o := range older {
		for _, s := range o.Services {
			if !have[s.Name] {
				return false
			}
		}
	}
	return true
}

func newService(sd protoreflect.ServiceDescriptor) *Service {
	svc := &Service{
		Name:     string(sd.Name()),
		FullName: string(sd.FullName()),
		Comment:  firstSentence(sd.ParentFile().SourceLocations().ByDescriptor(sd).LeadingComments),
	}
	if opts := reparse(sd.Options(), &descriptorpb.ServiceOptions{}); opts != nil {
		if h, ok := proto.GetExtension(opts, annotations.E_DefaultHost).(string); ok {
			svc.DefaultHost = h
		}
	}
	for i := range sd.Methods().Len() {
		md := sd.Methods().Get(i)
		m := &Method{
			Name:            string(md.Name()),
			FullMethod:      "/" + string(sd.FullName()) + "/" + string(md.Name()),
			ClientStreaming: md.IsStreamingClient(),
			ServerStreaming: md.IsStreamingServer(),
			Pagination:      paginationOf(md.Input()),
		}
		if opts := reparse(md.Options(), &descriptorpb.MethodOptions{}); opts != nil {
			if rule, ok := proto.GetExtension(opts, annotations.E_Http).(*annotations.HttpRule); ok && rule != nil {
				m.HTTPMethod, m.HTTPPath = httpRule(rule)
			}
			if mo, ok := opts.(*descriptorpb.MethodOptions); ok &&
				mo.GetIdempotencyLevel() != descriptorpb.MethodOptions_IDEMPOTENCY_UNKNOWN {
				m.Idempotent = true
			}
		}
		// Read-only by HTTP verb, or by Kentik's naming convention (many read-only List and Get RPCs are
		// mapped to POST because they take a request body, e.g. alerting List and GetResultsForTests).
		if m.HTTPMethod == "GET" || strings.HasPrefix(m.Name, "Get") || strings.HasPrefix(m.Name, "List") {
			m.Idempotent = !m.ClientStreaming
		}
		svc.Methods = append(svc.Methods, m)
	}
	return svc
}

// reparse round-trips options through the wire format so that extensions resolve to the generated Go
// types registered by the annotations package.
func reparse(opts proto.Message, into proto.Message) proto.Message {
	if opts == nil {
		return nil
	}
	b, err := proto.Marshal(opts)
	if err != nil {
		return nil
	}
	if err := (proto.UnmarshalOptions{Resolver: protoregistry.GlobalTypes}).Unmarshal(b, into); err != nil {
		return nil
	}
	return into
}

func httpRule(r *annotations.HttpRule) (method, path string) {
	switch p := r.GetPattern().(type) {
	case *annotations.HttpRule_Get:
		return "GET", p.Get
	case *annotations.HttpRule_Post:
		return "POST", p.Post
	case *annotations.HttpRule_Put:
		return "PUT", p.Put
	case *annotations.HttpRule_Patch:
		return "PATCH", p.Patch
	case *annotations.HttpRule_Delete:
		return "DELETE", p.Delete
	case *annotations.HttpRule_Custom:
		return strings.ToUpper(p.Custom.GetKind()), p.Custom.GetPath()
	default:
		return "", ""
	}
}

func paginationOf(in protoreflect.MessageDescriptor) string {
	fields := in.Fields()
	if f := fields.ByName("pagination"); f != nil && f.Message() != nil &&
		f.Message().Name() == "PaginationConfig" {
		return pagingConfig
	}
	isInt := func(f protoreflect.FieldDescriptor) bool {
		if f == nil || f.Cardinality() == protoreflect.Repeated {
			return false
		}
		switch f.Kind() {
		case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Uint32Kind, protoreflect.Uint64Kind:
			return true
		default:
			return false
		}
	}
	if isInt(fields.ByName("offset")) && isInt(fields.ByName("limit")) {
		return pagingOffset
	}
	return pagingNone
}

func firstSentence(comment string) string {
	c := strings.TrimSpace(comment)
	if c == "" {
		return ""
	}
	c = strings.Join(strings.Fields(strings.SplitN(c, "\n\n", 2)[0]), " ")
	if i := strings.Index(c, ". "); i >= 0 {
		c = c[:i+1]
	}
	if strings.ContainsAny(c, "{}") { // template placeholders such as {{.Name}}
		return ""
	}
	return c
}
