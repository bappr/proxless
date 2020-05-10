package inmemory

import (
	"fmt"
	"kube-proxless/internal/model"
	"kube-proxless/internal/utils"
	"testing"
)

// we volontarily do not create the inMemoryStore globally so that each test are independent from each other

type upsertTestCaseStruct struct {
	id, svc, port, deploy, ns string
	domains                   []string
	errWanted                 bool
}

func TestInMemoryStore_UpsertStore_Create(t *testing.T) {
	s := NewInMemoryStore()

	// create route
	testCases := []upsertTestCaseStruct{
		{"createTestCase0", "svc0", "80", "deploy0", "ns0", []string{"example.0"}, false},
		{"createTestCase1", "svc1", "", "deploy1", "ns1", []string{"example.1"}, false},
		{"", "svc", "80", "deploy2", "ns2", []string{"example.2"}, true},
		{"createTestCase2", "", "80", "deploy3", "ns3", []string{"example.3"}, true},
		{"createTestCase3", "svc3", "80", "", "ns4", []string{"example.4"}, true},
		{"createTestCase4", "svc4", "80", "deploy4", "", []string{"example5"}, true},
		{"createTestCase5", "svc5", "80", "deploy5", "ns5", nil, true},
	}

	upsertStoreHelper(testCases, t, s)
}

func TestInMemoryStore_UpsertStore_Update(t *testing.T) {
	s := NewInMemoryStore()

	testCases := []upsertTestCaseStruct{
		{"updateTestCase0", "svc0", "80", "deploy0", "ns", []string{"example.0.0"}, false},
		{"updateTestCase0", "svc0", "80", "deploy0", "ns", []string{"example.0.0"}, false},
		{"updateTestCase0", "svc0", "80", "deploy0.1", "ns", []string{"example.0.0", "example.0.1"}, false},
		{"updateTestCase1", "svc1", "", "deploy1", "ns1", []string{"example.1.0"}, false},
		{"updateTestCase1", "svc1", "8080", "deploy1", "ns1", []string{"example.1.0"}, false},
		{"updateTestCase1", "svc1", "", "deploy1", "ns1", []string{"example.1.0"}, true},
		{"updateTestCase2", "svc2", "8080", "deploy2", "ns1", []string{"example.1.0"}, true},
		{"updateTestCase2", "svc2", "8080", "deploy2", "ns1", []string{"example.1.1"}, false},
	}

	upsertStoreHelper(testCases, t, s)
}

func TestInMemoryStore_genDeploymentKey(t *testing.T) {
	deploy := "exampledeploy"
	ns := "examplens"
	want := fmt.Sprintf("%s.%s", deploy, ns)

	deploymentKey := genDeploymentKey(deploy, ns)

	if deploymentKey != want {
		t.Errorf("genDeploymentKey(%s, %s) = %s; want = %s", deploy, ns, deploymentKey, want)
	}
}

func TestInMemoryStore_CheckDeployAndDomainsOwnership(t *testing.T) {
	s := NewInMemoryStore()

	_ = s.UpsertStore("0", "svc0", "", "deploy0", "ns0", []string{"example.0.0"})

	testCases := []struct {
		id, deploy, ns string
		domains        []string
		errWanted      bool
	}{
		{"0", "deploy0", "ns0", []string{"example.0.0"}, false},
		{"0", "deploy0", "ns0", []string{"example.0.0"}, false},
		{"0", "deploy0", "ns0", []string{"example.0.1"}, false},
		{"1", "deploy1", "ns1", []string{"example.0.0"}, true},
		{"1", "deploy0", "ns0", []string{"example.1.0"}, true},
	}

	for _, tc := range testCases {
		errGot := checkDeployAndDomainsOwnership(s, tc.id, tc.deploy, tc.ns, tc.domains)

		if tc.errWanted != (errGot != nil) {
			t.Errorf("checkDeployAndDomainsOwnership(%s, %s ,%s, %s) = %v, errWanted = %t",
				tc.id, tc.deploy, tc.ns, tc.domains, errGot, tc.errWanted)
		}
	}
}

func TestInMemoryStore_cleanOldDeploymentFromStore(t *testing.T) {
	s := NewInMemoryStore()

	r0, _ := model.NewRoute("0", "svc0", "", "deploy0", "ns0", []string{"example.0.0"})
	createRoute(s, r0)

	testCases := []struct {
		id      string
		route   *model.Route
		domains []string
		want    []string
	}{
		{"0", r0, r0.GetDomains(), []string{}},
		{"1", r0, []string{"example.0.0", "example.0.1"}, []string{"example.0.1"}},
		{"2", r0, []string{"example.0.0", "example.0.1"}, []string{"example.0.1"}},
		{"3", r0, []string{"example.0.1"}, []string{"example.0.1"}},
		{"4", r0, []string{"example.0.0"}, []string{}}, // the store has been updated but the route did not change
	}

	for _, tc := range testCases {
		got := cleanOldDomainsFromStore(s, tc.route.GetDomains(), tc.domains)

		if !utils.CompareUnorderedArray(got, tc.want) {
			t.Errorf("cleanOldDeploymentFromStore(id = %s, %s) = %s; want = %s", tc.id, tc.domains, got, tc.want)
		}
	}
}

func TestInMemoryStore_UpdateLastUse(t *testing.T) {
	s := NewInMemoryStore()

	r0, _ := model.NewRoute("0", "svc0", "", "deploy0", "ns0", []string{"example.0.0"})
	createRoute(s, r0)

	lastUsed := r0.GetLastUsed()

	testCases := []struct {
		domain    string
		errWanted bool
	}{
		{r0.GetDomains()[0], false},
		{"", true},
	}

	for _, tc := range testCases {
		errGot := s.UpdateLastUse(tc.domain)

		if tc.errWanted != (errGot != nil) {
			t.Errorf("UpdateLastUse(%s) = %v; errWanted = %t", tc.domain, errGot, tc.errWanted)
		}

		if errGot == nil && !lastUsed.Before(r0.GetLastUsed()) {
			t.Errorf("UpdateLastUse(%s) - %s is not before %s", tc.domain, lastUsed, r0.GetLastUsed())
		}
	}
}

func TestInMemoryStore_DeleteRoute(t *testing.T) {
	s := NewInMemoryStore()

	r0, _ := model.NewRoute("0", "svc0", "", "deploy0", "ns0", []string{"example.0.0"})
	createRoute(s, r0)

	testCases := []struct {
		id        string
		errWanted bool
	}{
		{r0.GetId(), false},
		{r0.GetId(), true},
	}

	for _, tc := range testCases {
		errGot := s.DeleteRoute(tc.id)

		if tc.errWanted != (errGot != nil) {
			t.Errorf("DeleteRoute(%s) = %v; errWanted = %t", tc.id, errGot, tc.errWanted)
		}

		if errGot == nil {
			_, err := getRoute(s, tc.id)

			if err == nil {
				t.Errorf("DeleteRoute(%s) = %v; route still in store", tc.id, errGot)
			}
		}
	}
}
