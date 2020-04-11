package inmemory

import (
	"errors"
	"fmt"
	"kube-proxless/internal/model"
	"kube-proxless/internal/utils"
	"sync"
	"time"
)

type InMemoryStore struct {
	m    map[string]*model.Route
	lock sync.RWMutex
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		m:    make(map[string]*model.Route),
		lock: sync.RWMutex{},
	}
}

func (s *InMemoryStore) UpsertStore(id, service, port, deploy, namespace string, domains []string) error {
	if id == "" || service == "" || deploy == "" || namespace == "" || utils.IsArrayEmpty(domains) {
		return errors.New(
			fmt.Sprintf(
				"Error upserting route - id = %s, svc = %s, deploy = %s, ns = %s, domains = %s - must not be empty",
				id, service, deploy, namespace, domains),
		)
	}

	// error if deployment or domains are already associated to another route
	err := s.checkDeployAndDomainsOwnership(id, deploy, namespace, domains)

	if err != nil {
		return err
	}

	if existingRoute, ok := s.m[id]; ok {
		if port == "" {
			return errors.New(
				fmt.Sprintf(
					"Error updating route - port = %s must not be empty", port),
			)
		}

		// /!\ this need to be on top - otherwise the data will have already been overriden
		newKeys := s.cleanStore(deploy, namespace, domains, existingRoute)

		// associate the route to new deployment key / domains
		for _, k := range newKeys {
			s.m[k] = existingRoute
		}

		// TODO check the errors
		_ = existingRoute.SetService(service)
		_ = existingRoute.SetPort(port)
		_ = existingRoute.SetDeployment(deploy)
		_ = existingRoute.SetDomains(domains)
		// route is a pointer and it's changing dynamically - no need to "persist" the change in the map
	} else {
		newRoute, err := model.NewRoute(id, service, port, deploy, namespace, domains)

		if err != nil {
			return err
		}

		s.createRoute(newRoute)
	}

	return nil
}

// return an error if deploy or domains are already associated to a different id
func (s *InMemoryStore) checkDeployAndDomainsOwnership(id, deploy, ns string, domains []string) error {
	r, err := s.GetRouteByDeployment(deploy, ns)

	if err == nil && r.GetId() != id {
		return errors.New(fmt.Sprintf("Deployment %s.%s is already owned by %s", deploy, ns, r.GetId()))
	}

	for _, d := range domains {
		r, err = s.GetRouteByDomain(d)

		if err == nil && r.GetId() != id {
			return errors.New(fmt.Sprintf("Domain %s is already owned by %s", d, r.GetId()))
		}
	}

	return nil
}

func (s *InMemoryStore) createRoute(route *model.Route) {
	s.lock.Lock()
	defer s.lock.Unlock()

	deploymentKey := genDeploymentKey(route.GetDeployment(), route.GetNamespace())
	s.m[route.GetId()] = route
	s.m[deploymentKey] = route
	for _, d := range route.GetDomains() {
		s.m[d] = route
	}
}

// Remove domains and deployment from the existing route if they are not in the new route anymore
func (s *InMemoryStore) cleanStore(newDeploy, newNs string, newDomains []string, existingRoute *model.Route) []string {
	s.lock.Lock()
	defer s.lock.Unlock()

	var newKeys []string

	deployKeyNotInStore := s.cleanDeployment(existingRoute, newDeploy, newNs)

	if deployKeyNotInStore != "" {
		newKeys = append(newKeys, deployKeyNotInStore)
	}

	domainsNotInStore := s.cleanDomains(existingRoute, newDomains)

	if newDomains != nil {
		newKeys = append(newKeys, domainsNotInStore...)
	}

	if newKeys == nil {
		return []string{}
	}

	return newKeys
}

// return the new deployment key if it does not exist in the store
func (s *InMemoryStore) cleanDeployment(existingRoute *model.Route, newDeploy, newNs string) string {
	oldDeploymentKey := genDeploymentKey(existingRoute.GetDeployment(), existingRoute.GetNamespace())

	newDeploymentKey := genDeploymentKey(newDeploy, newNs)
	if oldDeploymentKey != newDeploymentKey {
		delete(s.m, oldDeploymentKey)
		return newDeploymentKey
	}

	return ""
}

// TODO review complexity
// return the new domains from the list who do not exist in the store
func (s *InMemoryStore) cleanDomains(existingRoute *model.Route, newDomains []string) []string {
	oldDomains := existingRoute.GetDomains()

	// get the difference between the 2 domains arrays
	diff := utils.DiffUnorderedArray(oldDomains, newDomains)

	var newKeys []string

	if diff != nil && len(diff) > 0 {
		// remove domain from the store if they are not in the list of new Domains
		for _, d := range diff {
			if !utils.Contains(newDomains, d) {
				delete(s.m, d)
			} else {
				newKeys = append(newKeys, d)
			}
		}
	}

	if newKeys == nil {
		return []string{}
	}

	return newKeys
}

func genDeploymentKey(deployment, namespace string) string {
	return fmt.Sprintf("%s.%s", deployment, namespace)
}

func (s *InMemoryStore) GetRouteByDomain(domain string) (*model.Route, error) {
	return s.getRoute(domain)
}

func (s *InMemoryStore) GetRouteByDeployment(deploy, namespace string) (*model.Route, error) {
	deploymentKey := genDeploymentKey(deploy, namespace)
	return s.getRoute(deploymentKey)
}

func (s *InMemoryStore) getRoute(key string) (*model.Route, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if route, ok := s.m[key]; ok {
		return route, nil
	}

	return nil, errors.New(fmt.Sprintf("Route %s not found in store", key))
}

func (s *InMemoryStore) UpdateLastUse(domain string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if route, ok := s.m[domain]; ok {
		route.SetLastUsed(time.Now())
		return nil
	}

	return errors.New(fmt.Sprintf("Route %s not found in store", domain))
}

func (s *InMemoryStore) DeleteRoute(id string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if route, ok := s.m[id]; ok {
		deploymentKey := genDeploymentKey(route.GetDeployment(), route.GetNamespace())
		delete(s.m, route.GetId())
		delete(s.m, deploymentKey)
		for _, d := range route.GetDomains() {
			delete(s.m, d)
		}
		return nil
	}

	return errors.New(fmt.Sprintf("Route %s not found in store", id))
}