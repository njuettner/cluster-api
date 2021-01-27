/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	clusterv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/cmd/version"
	expv1alpha4 "sigs.k8s.io/cluster-api/exp/api/v1alpha4"
	kubeadmbootstrapv1alpha4 "sigs.k8s.io/cluster-api/exp/kubeadm-ignition/api/v1alpha4"
	kubeadmbootstrapcontrollers "sigs.k8s.io/cluster-api/exp/kubeadm-ignition/controllers"
	ignition "sigs.k8s.io/cluster-api/exp/kubeadm-ignition/internal/ignition"
	"sigs.k8s.io/cluster-api/feature"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	klog.InitFlags(nil)

	_ = clientgoscheme.AddToScheme(scheme)
	_ = clusterv1alpha4.AddToScheme(scheme)
	_ = expv1alpha4.AddToScheme(scheme)
	_ = kubeadmbootstrapv1alpha4.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

var (
	metricsBindAddr             string
	enableLeaderElection        bool
	leaderElectionLeaseDuration time.Duration
	leaderElectionRenewDeadline time.Duration
	leaderElectionRetryPeriod   time.Duration
	watchNamespace              string
	profilerAddress             string
	kubeadmConfigConcurrency    int
	syncPeriod                  time.Duration
	webhookPort                 int
	userDataBucket              string
	userdataDir                 string
)

func InitFlags(fs *pflag.FlagSet) {
	fs.StringVar(&metricsBindAddr, "metrics-bind-addr", ":8080",
		"The address the metric endpoint binds to.")

	fs.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")

	fs.DurationVar(&leaderElectionLeaseDuration, "leader-election-lease-duration", 15*time.Second,
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)")

	fs.DurationVar(&leaderElectionRenewDeadline, "leader-election-renew-deadline", 10*time.Second,
		"Duration that the leading controller manager will retry refreshing leadership before giving up (duration string)")

	fs.DurationVar(&leaderElectionRetryPeriod, "leader-election-retry-period", 2*time.Second,
		"Duration the LeaderElector clients should wait between tries of actions (duration string)")

	fs.StringVar(&watchNamespace, "namespace", "",
		"Namespace that the controller watches to reconcile cluster-api objects. If unspecified, the controller watches for cluster-api objects across all namespaces.")

	fs.StringVar(&profilerAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")

	fs.IntVar(&kubeadmConfigConcurrency, "kubeadmconfig-concurrency", 10,
		"Number of kubeadm configs to process simultaneously")

	fs.DurationVar(&syncPeriod, "sync-period", 10*time.Minute,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")

	fs.DurationVar(&kubeadmbootstrapcontrollers.DefaultTokenTTL, "bootstrap-token-ttl", 15*time.Minute,
		"The amount of time the bootstrap token will be valid")

	fs.IntVar(&webhookPort, "webhook-port", 0,
		"Webhook Server port, disabled by default. When enabled, the manager will only work as webhook server, no reconcilers are installed.")

	flag.StringVar(
		&userDataBucket,
		"ignition-userdata-bucket",
		"container-service-demo",
		"The bucket the userdata ignition file resides",
	)
	flag.StringVar(
		&userdataDir,
		"ignition-userdata-dir",
		"node-userdata",
		"The bucket the userdata ignition file resides",
	)

	feature.MutableGates.AddFlag(fs)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	InitFlags(pflag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(klogr.New())

	if profilerAddress != "" {
		klog.Infof("Profiler listening for requests at %s", profilerAddress)
		go func() {
			klog.Info(http.ListenAndServe(profilerAddress, nil))
		}()
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsBindAddr,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "kubeadm-bootstrap-manager-leader-election-capi",
		LeaseDuration:      &leaderElectionLeaseDuration,
		RenewDeadline:      &leaderElectionRenewDeadline,
		RetryPeriod:        &leaderElectionRetryPeriod,
		Namespace:          watchNamespace,
		SyncPeriod:         &syncPeriod,
		NewClient:          newClientFunc,
		Port:               webhookPort,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	templateBackend, err := ignition.NewS3TemplateBackend(userdataDir, userDataBucket)
	if err != nil {
		setupLog.Error(err, "unable to create aws s3 session")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	setupWebhooks(mgr)
	setupReconcilers(ctx, mgr, templateBackend)

	// +kubebuilder:scaffold:builder
	setupLog.Info("starting manager", "version", version.Get().String())
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setupReconcilers(ctx context.Context, mgr ctrl.Manager, templateBackend ignition.TemplateBackend) {
	if webhookPort != 0 {
		return
	}

	if err := (&kubeadmbootstrapcontrollers.KubeadmIgnitionConfigReconciler{
		Client:          mgr.GetClient(),
		Log:             ctrl.Log.WithName("controllers").WithName("KubeadmIgnitionConfigIgnition"),
		IgnitionFactory: ignition.NewFactory(templateBackend),
	}).SetupWithManager(ctx, mgr, concurrency(kubeadmConfigConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KubeadmIgnitionConfigIgnition")
		os.Exit(1)
	}
}

func setupWebhooks(mgr ctrl.Manager) {
	if webhookPort == 0 {
		return
	}

	if err := (&kubeadmbootstrapv1alpha4.KubeadmIgnitionConfig{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "KubeadmIgnitionConfig")
		os.Exit(1)
	}
	if err := (&kubeadmbootstrapv1alpha4.KubeadmIgnitionConfigList{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "KubeadmIgnitionConfigList")
		os.Exit(1)
	}
	if err := (&kubeadmbootstrapv1alpha4.KubeadmIgnitionConfigTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "KubeadmIgnitionConfigTemplate")
		os.Exit(1)
	}
	if err := (&kubeadmbootstrapv1alpha4.KubeadmIgnitionConfigTemplateList{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "KubeadmIgnitionConfigTemplateList")
		os.Exit(1)
	}
}

func concurrency(c int) controller.Options {
	return controller.Options{MaxConcurrentReconciles: c}
}

// newClientFunc returns a client reads from cache and write directly to the server
// this avoid get unstructured object directly from the server
// see issue: https://github.com/kubernetes-sigs/cluster-api/issues/1663
func newClientFunc(cache cache.Cache, config *rest.Config, options client.Options) (client.Client, error) {
	// Create the Client for Write operations.
	c, err := client.New(config, options)
	if err != nil {
		return nil, err
	}

	return client.NewDelegatingClient(client.NewDelegatingClientInput{
		CacheReader: c,
		Client:      c,
	}), nil
}