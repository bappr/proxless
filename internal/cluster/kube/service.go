package kube

import (
	"errors"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"kube-proxless/internal/cluster"
	"kube-proxless/internal/logger"
	"strconv"
)

func createProxlessService(
	clientSet kubernetes.Interface, appSvc, appNs, proxlessSvc, proxlessNs string) (*corev1.Service, error) {
	svc, err := clientSet.CoreV1().Services(appNs).Create(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        genServiceToAppName(appSvc),
			Annotations: map[string]string{"owner": "proxless"},
		},
		Spec: corev1.ServiceSpec{
			Type:         "ExternalName",
			ExternalName: fmt.Sprintf("%s.%s.svc.cluster.local", proxlessSvc, proxlessNs),
		},
	})

	// not an issue if proxless service already exists and we don't wanna update it
	if k8serrors.IsAlreadyExists(err) {
		return svc, nil
	}

	return svc, err
}

func deleteProxlessService(clientSet kubernetes.Interface, appSvc, appNs string) error {
	return clientSet.CoreV1().Services(appNs).Delete(genServiceToAppName(appSvc), &metav1.DeleteOptions{})
}

func parseService(obj interface{}) (*corev1.Service, error) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return nil, errors.New(fmt.Sprintf("event for invalid object; got %T want *core.Service", obj))
	}
	return svc, nil
}

func getPortFromServicePorts(ports []corev1.ServicePort) string {
	port := ports[0] // TODO add possibility to manage multiple ports

	return strconv.Itoa(int(port.TargetPort.IntVal))
}

func addServiceToStore(
	clientset kubernetes.Interface, svc *corev1.Service, namespaceScoped bool,
	proxlessSvc, proxlessNamespace string,
	upsertStore func(id, name, port, deployName, namespace string, domains []string) error,
) {
	if isAnnotationsProxlessCompatible(svc.ObjectMeta) {
		deployName := svc.Annotations[cluster.AnnotationServiceDeployKey]

		_, err := createProxlessService(clientset, svc.Name, svc.Namespace, proxlessSvc, proxlessNamespace)

		if err != nil {
			logger.Errorf(err, "Error creating proxless service for %s.%s", svc.Name, svc.Namespace)
			// do not return here - we don't wanna break the proxy forwarding
			// it will be relabel after the informer resync
		}

		_, err = labelDeployment(clientset, deployName, svc.Namespace)

		if err != nil {
			logger.Errorf(err, "Error labelling deployment %s.%s", deployName, svc.Namespace)
			// do not return here - we don't wanna break the proxy forwarding
			// it will be relabel after the informer resync
		}

		port := getPortFromServicePorts(svc.Spec.Ports)
		domains :=
			genDomains(svc.Annotations[cluster.AnnotationServiceDomainKey], svc.Name, svc.Namespace, namespaceScoped)

		err = upsertStore(string(svc.UID), svc.Name, port, deployName, svc.Namespace, domains)

		if err == nil {
			logger.Debugf("Service %s.%s added into the store", svc.Name, svc.Namespace)
		} else {
			logger.Errorf(err, "Error adding service %s.%s into the store", svc.Name, svc.Namespace)
		}
	}
}

func removeServiceFromStore(
	clientset kubernetes.Interface, svc *corev1.Service,
	deleteRouteFromStore func(id string) error,
) {
	if isAnnotationsProxlessCompatible(svc.ObjectMeta) {
		deployName := svc.Annotations[cluster.AnnotationServiceDeployKey]

		// we don't process the error here - the deployment might have been delete with the service
		_, _ = removeDeploymentLabel(clientset, deployName, svc.Namespace)

		_ = deleteProxlessService(clientset, svc.Name, svc.Namespace)

		err := deleteRouteFromStore(string(svc.UID))

		if err == nil {
			logger.Debugf("Service %s.%s removed from the store", svc.Name, svc.Namespace)
		} else {
			logger.Errorf(err, "Error removing service %s.%s from store", svc.Name, svc.Namespace)
		}
	}
}

func updateServiceInStore(
	clientset kubernetes.Interface, oldSvc, newSvc *corev1.Service, namespaceScoped bool,
	proxlessService, proxlessNamespace string,
	upsertStore func(id, name, port, deployName, namespace string, domains []string) error,
	deleteRouteFromStore func(id string) error,
) {
	if isAnnotationsProxlessCompatible(oldSvc.ObjectMeta) &&
		isAnnotationsProxlessCompatible(newSvc.ObjectMeta) { // updating service
		oldDeployName := oldSvc.Annotations[cluster.AnnotationServiceDeployKey]

		if oldDeployName != newSvc.Annotations[cluster.AnnotationServiceDeployKey] {
			_, err := removeDeploymentLabel(clientset, oldDeployName, oldSvc.Namespace)
			if err != nil {
				logger.Errorf(err, "error remove proxless label from deployment %s.%s",
					oldDeployName, oldSvc.Namespace)
			}
		}

		// the `addServiceToStore` is idempotent so we can reuse it in the update
		addServiceToStore(clientset, newSvc, namespaceScoped, proxlessService, proxlessNamespace, upsertStore)
	} else if !isAnnotationsProxlessCompatible(oldSvc.ObjectMeta) &&
		isAnnotationsProxlessCompatible(newSvc.ObjectMeta) { // adding new service
		addServiceToStore(clientset, newSvc, namespaceScoped, proxlessService, proxlessNamespace, upsertStore)
	} else if isAnnotationsProxlessCompatible(oldSvc.ObjectMeta) &&
		!isAnnotationsProxlessCompatible(newSvc.ObjectMeta) { // removing service
		removeServiceFromStore(clientset, oldSvc, deleteRouteFromStore)
	}
}
