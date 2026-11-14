package controller

import (
	"fmt"
	"strings"

	operatorv1 "github.com/openshift/api/operator/v1"
)

func renderDNSTemplateStanzas(config operatorv1.DNSTemplate) (string, error) {
	if len(config.Zones) == 0 {
		return "", nil
	}
	if string(config.QueryClass) != "IN" || string(config.QueryType) != "AAAA" || string(config.Action.Rcode) != "NOERROR" {
		return "", fmt.Errorf("unsupported DNS template class %q, type %q, or response code %q", config.QueryClass, config.QueryType, config.Action.Rcode)
	}
	var b strings.Builder
	for _, zone := range config.Zones {
		if zone == "" || strings.ContainsAny(string(zone), " \t\n\r{}\"") {
			return "", fmt.Errorf("invalid DNS template zone %q", zone)
		}
		fmt.Fprintf(&b, "    template IN AAAA %s {\n        rcode NOERROR\n        fallthrough\n    }\n", zone)
	}
	return b.String(), nil
}
