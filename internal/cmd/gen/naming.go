package main

import (
	"strings"
	"unicode"
)

// initialisms are rendered upper case in Go identifiers.
var initialisms = map[string]string{
	"ai": "AI", "as": "AS", "bgp": "BGP", "kmi": "KMI", "mkp": "MKP", "nms": "NMS",
	"kptr": "KPTR", "ktbgp": "KTBGP", "api": "API", "id": "ID", "deviceconf": "DeviceConf",
}

// camel converts "cloud_export" to "CloudExport" and "alerting/public" to "AlertingPublic".
func camel(s string) string {
	var b strings.Builder
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '/' || r == '-' }) {
		if up, ok := initialisms[part]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

// versionIdent converts "v202309" to "V202309" and "v202504beta2" to "V202504Beta2".
func versionIdent(v string) string {
	var b strings.Builder
	prevDigit := false
	for i, r := range v {
		switch {
		case i == 0:
			b.WriteRune(unicode.ToUpper(r))
		case unicode.IsLetter(r) && prevDigit:
			b.WriteRune(unicode.ToUpper(r))
		default:
			b.WriteRune(r)
		}
		prevDigit = unicode.IsDigit(r)
	}
	return b.String()
}

// accessorName is the method name for a version, e.g. "SyntheticsV202309".
func accessorName(v *Version) string { return camel(v.API.Path) + versionIdent(v.Name) }

// aliasName is the unversioned alias, e.g. "Synthetics".
func aliasName(a *API) string { return camel(a.Path) }

// groupTypeName is the struct returned by multi-service accessors, e.g. "SyntheticsV202309Clients".
func groupTypeName(v *Version) string { return accessorName(v) + "Clients" }

// importAlias is the Go import alias, e.g. "alertingpublicv202505".
func importAlias(v *Version) string {
	return strings.NewReplacer("_", "", "/", "").Replace(v.API.Path) + v.Name
}

// fieldName is the struct field for a service within a multi-service version: the API prefix and the
// "Service" suffix are dropped (SyntheticsAdminService -> Admin, AlertService -> Alert).
func fieldName(apiPath, service string) string {
	name := strings.TrimSuffix(service, "Service")
	prefix := strings.ToLower(strings.NewReplacer("_", "", "/", "").Replace(apiPath))
	if len(name) > len(prefix) && strings.HasPrefix(strings.ToLower(name), prefix) {
		rest := name[len(prefix):]
		if r := rune(rest[0]); unicode.IsUpper(r) {
			name = rest
		}
	}
	return name
}
