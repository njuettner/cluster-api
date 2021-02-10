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
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *KubeadmControlPlane) ConvertTo(destRaw conversion.Hub) error {
	dest := destRaw.(*v1alpha4.KubeadmControlPlane)
	if err := Convert_v1alpha3_KubeadmControlPlane_To_v1alpha4_KubeadmControlPlane(src, dest, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &v1alpha4.KubeadmControlPlane{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dest.Spec.KubeadmConfigSpec.Ignition = restored.Spec.KubeadmConfigSpec.Ignition

	return nil
}

func (dest *KubeadmControlPlane) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.KubeadmControlPlane)
	if err := Convert_v1alpha4_KubeadmControlPlane_To_v1alpha3_KubeadmControlPlane(src, dest, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata.
	if err := utilconversion.MarshalData(src, dest); err != nil {
		return err
	}

	return nil
}

func (src *KubeadmControlPlaneList) ConvertTo(destRaw conversion.Hub) error {
	dest := destRaw.(*v1alpha4.KubeadmControlPlaneList)
	return Convert_v1alpha3_KubeadmControlPlaneList_To_v1alpha4_KubeadmControlPlaneList(src, dest, nil)
}

func (dest *KubeadmControlPlaneList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha4.KubeadmControlPlaneList)
	return Convert_v1alpha4_KubeadmControlPlaneList_To_v1alpha3_KubeadmControlPlaneList(src, dest, nil)
}
