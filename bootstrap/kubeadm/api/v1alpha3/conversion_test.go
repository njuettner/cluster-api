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

package v1alpha3

import (
	"math/rand"
	"testing"

	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/gomega"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha4.AddToScheme(scheme)).To(Succeed())

	t.Run("for KubeadmConfig", utilconversion.FuzzTestFunc(scheme, &v1alpha4.KubeadmConfig{}, &KubeadmConfig{}, KubeadmConfigStatusFuzzFuncs, kubeadmFuzzerFuncs))
	t.Run("for KubeadmConfigTemplate", utilconversion.FuzzTestFunc(scheme, &v1alpha4.KubeadmConfigTemplate{}, &KubeadmConfigTemplate{}, kubeadmFuzzerFuncs))
}

func KubeadmConfigStatusFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		KubeadmConfigStatusFuzzer,
	}
}

func KubeadmConfigStatusFuzzer(obj *KubeadmConfigStatus, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	// KubeadmConfigStatus.BootstrapData has been removed in v1alpha4, so setting it to nil in order to avoid v1alpha3 --> v1alpha4 --> v1alpha3 round trip errors.
	obj.BootstrapData = nil
}

func kubeadmFuzzerFuncs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		// Fuzzer for BootstrapToken to ensure correctness of the token format.
		func(j **kubeadmv1beta1.BootstrapTokenString, c fuzz.Continue) {
			if c.RandBool() {
				t := &kubeadmv1beta1.BootstrapTokenString{}
				c.Fuzz(t)

				t.ID = randTokenString(6)
				t.Secret = randTokenString(16)

				*j = t
			} else {
				*j = nil
			}
		},
	}
}

const tokenCharsBytes = "abcdefghijklmnopqrstuvwxyz0123456789"

func randTokenString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = tokenCharsBytes[rand.Intn(len(tokenCharsBytes))]
	}
	return string(b)
}
