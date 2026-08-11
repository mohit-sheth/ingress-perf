// Copyright 2024 The ingress-perf Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package runner

import (
	"bytes"
	"context"
	"strings"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

func getHAProxyVersion() (string, error) {
	podList, err := clientSet.CoreV1().Pods("openshift-ingress").List(context.TODO(),
		metav1.ListOptions{
			LabelSelector: "ingresscontroller.operator.openshift.io/deployment-ingresscontroller=default",
			FieldSelector: "status.phase=Running"},
	)
	if err != nil {
		return "", err
	}
	routerPod := podList.Items[0]
	rpmCmd := []string{"bash", "-c", "rpm -qa | grep haproxy"}

	// OCP 5.0+ deploys HAProxy as a sidecar container separate from the
	// router controller. Check for HAProxy RPM in the sidecar first.
	version, err := execInContainer(routerPod, "haproxy", rpmCmd)
	if err == nil && version != "" {
		return version, nil
	}
	log.Debugf("Failed to get HAProxy version from sidecar container, falling back to router container: %v", err)

	// Fall back to the monolithic router container (OCP 4.x).
	return execInContainer(routerPod, "router", rpmCmd)
}

func execInContainer(pod corev1.Pod, container string, command []string) (string, error) {
	var stdout, stderr bytes.Buffer
	req := clientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		Command:   command,
		TTY:       false,
	}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return "", err
	}
	err = exec.StreamWithContext(context.TODO(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}
