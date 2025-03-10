package migrate

import (
	"context"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/version"
	"github.com/tsuru/tsuru/db/storagev2"
	kubeProv "github.com/tsuru/tsuru/provision/kubernetes"
	tsuruv1clientset "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned"
	faketsuru "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/fake"
	kubeTesting "github.com/tsuru/tsuru/provision/kubernetes/testing"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/provision"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"golang.org/x/crypto/bcrypt"
	check "gopkg.in/check.v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type S struct {
	clusterClient *kubeProv.ClusterClient
	client        *kubeTesting.ClientWrapper
	cluster       *provision.Cluster
	mock          *kubeTesting.KubeMock
	mockService   servicemock.MockService
}

var _ = check.Suite(&S{})

func Test(t *testing.T) { check.TestingT(t) }

func (s *S) SetUpSuite(c *check.C) {
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "kubernetes_migrate_tests_s")
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake:default", true)
	var err error
	err = storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
	servicemock.SetMockService(&s.mockService)
	s.cluster = &provision.Cluster{
		Name:        "c1",
		Addresses:   []string{"https://clusteraddr"},
		Provisioner: "kubernetes",
		Pools:       []string{"kube", "test-default"},
	}
	s.clusterClient, err = kubeProv.NewClusterClient(s.cluster)
	c.Assert(err, check.IsNil)
	s.client = &kubeTesting.ClientWrapper{
		Clientset:              fake.NewSimpleClientset(),
		ClusterInterface:       s.clusterClient,
		ApiExtensionsClientset: fakeapiextensions.NewSimpleClientset(),
		TsuruClientset:         faketsuru.NewSimpleClientset(),
	}
	s.clusterClient.Interface = s.client
	kubeProv.ClientForConfig = func(conf *rest.Config) (kubernetes.Interface, error) {
		return s.client, nil
	}
	kubeProv.TsuruClientForConfig = func(conf *rest.Config) (tsuruv1clientset.Interface, error) {
		return s.client.TsuruClientset, nil
	}
	prov := kubeProv.GetProvisioner()
	s.mock = kubeTesting.NewKubeMock(s.client, prov, prov, nil)
	s.client.ApiExtensionsClientset.PrependReactor("create", "customresourcedefinitions", s.mock.CRDReaction(c))
	kubeProv.ExtensionsClientForConfig = func(conf *rest.Config) (apiextensionsclientset.Interface, error) {
		return s.client.ApiExtensionsClientset, nil
	}
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "test-default",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "kube",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "kube-failed",
		Provisioner: "kubernetes",
	})
	c.Assert(err, check.IsNil)
	err = pool.AddPool(context.TODO(), pool.AddPoolOptions{
		Name:        "docker",
		Provisioner: "docker",
	})
	c.Assert(err, check.IsNil)
}

func (s *S) SetUpTest(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.DeleteMany(context.TODO(), mongoBSON.M{})
	c.Assert(err, check.IsNil)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	for _, a := range appList.Items {
		err = s.client.TsuruV1().Apps("tsuru").Delete(context.TODO(), a.GetName(), metav1.DeleteOptions{})
		c.Assert(err, check.IsNil)
	}
	servicemanager.AppVersion, err = version.AppVersionService()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
}

func (s *S) TestMigrateAppsCRDs(c *check.C) {
	apps := []any{
		appTypes.App{Name: "app-kube", Pool: "kube"},
		appTypes.App{Name: "app-kube2", Pool: "kube"},
		appTypes.App{Name: "app-kube-failed", Pool: "kube-failed"},
		appTypes.App{Name: "app-docker", Pool: "docker"},
	}
	s.mockService.Cluster.OnFindByPool = func(prov, pool string) (*provision.Cluster, error) {
		if prov != s.cluster.Provisioner {
			return nil, provision.ErrNoCluster
		}
		for _, p := range s.cluster.Pools {
			if pool == p {
				return s.cluster, nil
			}
		}
		return nil, provision.ErrNoCluster
	}

	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	_, err = appsCollection.InsertMany(context.TODO(), apps)
	c.Assert(err, check.IsNil)

	err = MigrateAppsCRDs()
	c.Assert(err, check.NotNil)
	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(appList.Items), check.Equals, 2)
	c.Assert(appList.Items[0].Name, check.Equals, "app-kube")
	c.Assert(appList.Items[1].Name, check.Equals, "app-kube2")
}
