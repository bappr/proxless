package utils

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

func GenServiceToAppName(svcName string) string {
	return fmt.Sprintf("%s-proxless", svcName)
}

func GenDomains(domains, name, namespace string, namespaceScoped bool) []string {
	svcName := GenServiceToAppName(name)
	var domainsArray []string
	if domains != "" {
		domainsArray = strings.Split(domains, ",")
	}
	domainsArray = append(domainsArray, fmt.Sprintf("%s.%s", name, namespace))
	domainsArray = append(domainsArray, fmt.Sprintf("%s.%s", svcName, namespace))
	domainsArray = append(domainsArray, fmt.Sprintf("%s.%s.svc.cluster.local", name, namespace))
	domainsArray = append(domainsArray, fmt.Sprintf("%s.%s.svc.cluster.local", svcName, namespace))

	if namespaceScoped {
		domainsArray = append(domainsArray, name)
		domainsArray = append(domainsArray, svcName)
	}

	return domainsArray
}

func IsAnnotationsProxlessCompatible(meta metav1.ObjectMeta) bool {
	return metav1.HasAnnotation(meta, AnnotationServiceDeployKey)
}