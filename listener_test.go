// Copyright 2017 Yahoo Holdings Inc. 
// Licensed under the terms of the 3-Clause BSD License.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os/user"
	"testing"

	"k8s.io/api/admission/v1alpha1"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api"
	corev1 "k8s.io/client-go/pkg/api/v1"
	appsv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	autoscalingv1 "k8s.io/client-go/pkg/apis/autoscaling/v1"
	extensionsv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"

	"github.com/stretchr/testify/assert"
)

var (
	templateNamespace = &corev1.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name:            "test-namespace",
			ResourceVersion: "1",
		},
		Spec: corev1.NamespaceSpec{
			Finalizers: []corev1.FinalizerName{"kubernetes"},
		},
	}
	templateAdmReview = &v1alpha1.AdmissionReview{
		Spec: v1alpha1.AdmissionReviewSpec{
			Resource: v1.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
			},
			Kind: v1.GroupVersionKind{
				Kind: "Namespace",
			},
			Object: runtime.RawExtension{
				Raw: []byte("{}"),
			},
			Name:      "test-namespace",
			Namespace: "test-namespace",
			Operation: "DELETE",
			UserInfo: authenticationv1.UserInfo{
				Username: (func() string {
					user, err := user.Current()
					if err != nil {
						panic(err)
					}
					return user.Name
				})(),
			},
		},
	}
)

func cloneNamespace(templateNamespace *corev1.Namespace) *corev1.Namespace {
	testNamespaceObj, err := api.Scheme.DeepCopy(templateNamespace)
	testNamespace, ok := testNamespaceObj.(*corev1.Namespace)
	if err != nil || !ok {
		panic(fmt.Sprintf("Cloning Namespace failed with err: %v, ok: %t", err, ok))
	}
	return testNamespace
}

func cloneAdmissionReview(templateAdmReview *v1alpha1.AdmissionReview) *v1alpha1.AdmissionReview {
	testAdmReviewObj, err := api.Scheme.DeepCopy(templateAdmReview)
	testAdmReview, ok := testAdmReviewObj.(*v1alpha1.AdmissionReview)
	if err != nil || !ok {
		panic(fmt.Sprintf("Cloning test AdmissionReview spec failed with err: %v, ok: %t", err, ok))
	}
	return testAdmReview
}

func getAdmissionReview(rw *httptest.ResponseRecorder) *v1alpha1.AdmissionReview {
	admReview := &v1alpha1.AdmissionReview{}
	err := json.NewDecoder(rw.Result().Body).Decode(admReview)
	if err != nil {
		panic(err.Error())
	}
	return admReview
}

func constructPostBody(admReview *v1alpha1.AdmissionReview) io.Reader {
	body := new(bytes.Buffer)
	err := json.NewEncoder(body).Encode(admReview)
	if err != nil {
		panic(err.Error())
	}
	return body
}

func TestAllowedWriteResponse(t *testing.T) {
	rw := httptest.NewRecorder()
	review := &v1alpha1.AdmissionReview{}
	writeResponse(rw, review, true, "")

	admReview := getAdmissionReview(rw)

	expectedAdmReview := &v1alpha1.AdmissionReview{
		Status: v1alpha1.AdmissionReviewStatus{
			Allowed: true,
			Result: &v1.Status{
				Reason: v1.StatusReason(""),
			},
		},
	}
	assert.Equal(t,
		expectedAdmReview.Status,
		admReview.Status,
		"writeResponse should write Allowed: true for AdmissionReviewStatus")
}

func TestNotAllowedWriteResponse(t *testing.T) {
	rw := httptest.NewRecorder()
	review := &v1alpha1.AdmissionReview{}
	writeResponse(rw, review, false, "Namespace test-namespace contains one or more resources")

	admReview := getAdmissionReview(rw)

	expectedAdmReview := &v1alpha1.AdmissionReview{
		Status: v1alpha1.AdmissionReviewStatus{
			Allowed: false,
			Result: &v1.Status{
				Reason: v1.StatusReason("Namespace test-namespace contains one or more resources"),
			},
		},
	}
	assert.Equal(t,
		expectedAdmReview.Status,
		admReview.Status,
		"writeResponse should write Allowed: false for AdmissionReviewStatus")
}

func TestWrongMethodWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://localhost:8080/namespaces", nil)

	webhookHandler(rw, req)

	assert.Equal(t, rw.Code, 405)
}

func TestWrongPathWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://localhost:8080/namespaces", nil)

	webhookHandler(rw, req)

	assert.Equal(t, rw.Code, 404)
	body, err := ioutil.ReadAll(rw.Result().Body)
	assert.Nil(t, err, "Error should be nil")
	assert.Contains(t, string(body), "/namespaces 404 Not Found")
}

func TestWrongReqBodyWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://localhost:8080/", nil)

	webhookHandler(rw, req)

	admReview := getAdmissionReview(rw)

	assert.False(t, admReview.Status.Allowed, "should fail if request doesn't have a body")
	assert.Contains(t, admReview.Status.Result.Reason, "Failed to decode the request body json into an AdmissionReview resource: ")
}

func TestAdmitAllWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()

	testSpec := cloneAdmissionReview(templateAdmReview)

	*admitAll = true

	req := httptest.NewRequest("POST", "http://localhost:8080/", constructPostBody(testSpec))
	webhookHandler(rw, req)

	admReview := getAdmissionReview(rw)

	assert.True(t, admReview.Status.Allowed, "should allow namespace delete to pass through if admitAll flag is set")
	*admitAll = false
}

func TestNamespaceResourceTypeWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()

	testSpec := &v1alpha1.AdmissionReview{
		Spec: v1alpha1.AdmissionReviewSpec{
			Resource: v1.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
		},
	}

	req := httptest.NewRequest("POST", "http://localhost:8080/", constructPostBody(testSpec))
	webhookHandler(rw, req)

	admReview := getAdmissionReview(rw)

	assert.False(t, admReview.Status.Allowed, "should reject if the resource is not Namespace type")
	assert.Contains(t, admReview.Status.Result.Reason, "Incoming resource is not a Namespace: { v1 pods}")
}

func TestWrongOperationWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()

	testSpec := cloneAdmissionReview(templateAdmReview)

	testSpec.Spec.Operation = v1alpha1.Create

	req := httptest.NewRequest("POST", "http://localhost:8080/", constructPostBody(testSpec))
	webhookHandler(rw, req)

	admReview := getAdmissionReview(rw)

	assert.False(t, admReview.Status.Allowed, "should reject if the operation is NOT DELETE")
	assert.Contains(t, admReview.Status.Result.Reason, "Incoming operation is CREATE on namespace test-namespace. Only DELETE is currently supported.")
}

func TestNonExistingNamespaceWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()

	testSpec := cloneAdmissionReview(templateAdmReview)
	clientset = &fake.Clientset{}
	req := httptest.NewRequest("POST", "http://localhost:8080/", constructPostBody(testSpec))
	webhookHandler(rw, req)

	admReview := getAdmissionReview(rw)

	assert.True(t, admReview.Status.Allowed, "should approve if the namespace does not exist")
}

func TestBypassAnnotationTrueWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()

	testPod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
		Spec: corev1.PodSpec{
			Hostname: "test-pod.yahoo.com",
		},
	}
	testNamespace := cloneNamespace(templateNamespace)
	testNamespace.Annotations = map[string]string{bypassAnnotationKey: "true"}
	clientset = fake.NewSimpleClientset(testPod, testNamespace)

	testSpec := cloneAdmissionReview(templateAdmReview)
	req := httptest.NewRequest("POST", "http://localhost:8080/", constructPostBody(testSpec))

	webhookHandler(rw, req)

	admReview := getAdmissionReview(rw)

	assert.True(t, admReview.Status.Allowed, "should approve if the bypass annotation is set to true")
}

func TestBypassAnnotationFalseWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()

	testPod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
		Spec: corev1.PodSpec{
			Hostname: "test-pod.yahoo.com",
		},
	}
	testNamespace := cloneNamespace(templateNamespace)
	testNamespace.Annotations = map[string]string{bypassAnnotationKey: "false"}
	clientset = fake.NewSimpleClientset(testPod, testNamespace)

	testSpec := cloneAdmissionReview(templateAdmReview)
	req := httptest.NewRequest("POST", "http://localhost:8080/", constructPostBody(testSpec))

	webhookHandler(rw, req)

	admReview := getAdmissionReview(rw)

	assert.False(t, admReview.Status.Allowed, "should reject if the namespace has pod resources and bypass annotation is set to false")
	assert.Contains(t, admReview.Status.Result.Reason, "The namespace test-namespace you are trying to remove contains one or more of these resources: [pods(1)]. Please delete them and try again.")
}

func TestEmptyNamespaceWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()

	testNamespace := cloneNamespace(templateNamespace)
	clientset = fake.NewSimpleClientset(testNamespace)
	testSpec := cloneAdmissionReview(templateAdmReview)
	req := httptest.NewRequest("POST", "http://localhost:8080/", constructPostBody(testSpec))
	webhookHandler(rw, req)

	admReview := getAdmissionReview(rw)

	assert.True(t, admReview.Status.Allowed, "should approve if the namespace has no workload resources")
}

func TestNonEmptyNamespaceWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()

	testPod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
		Spec: corev1.PodSpec{
			Hostname: "test-pod.yahoo.com",
		},
	}
	testNamespace := cloneNamespace(templateNamespace)
	testSpec := cloneAdmissionReview(templateAdmReview)
	clientset = fake.NewSimpleClientset(testPod, testNamespace)
	req := httptest.NewRequest("POST", "http://localhost:8080/", constructPostBody(testSpec))
	webhookHandler(rw, req)

	admReview := getAdmissionReview(rw)

	assert.False(t, admReview.Status.Allowed, "should reject if the namespace has pod resources")
	assert.Contains(t, admReview.Status.Result.Reason, "The namespace test-namespace you are trying to remove contains one or more of these resources: [pods(1)]. Please delete them and try again.")
}

func TestNonEmptyNamespaceWithMoreResourcesWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()

	testPod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
		Spec: corev1.PodSpec{
			Hostname: "test-pod.yahoo.com",
		},
	}
	testSvc := &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-namespace",
		},
		Spec: corev1.ServiceSpec{
			ExternalName: "test-svc.yahoo.com",
		},
	}
	testReplicaSet := &extensionsv1beta1.ReplicaSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-replicaset",
			Namespace: "test-namespace",
		},
		Spec: extensionsv1beta1.ReplicaSetSpec{
			Replicas: new(int32),
		},
	}
	testDeployment := &appsv1beta1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "test-namespace",
		},
		Spec: appsv1beta1.DeploymentSpec{
			Replicas: new(int32),
		},
	}
	testStatefulSet := &appsv1beta1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "test-namespace",
		},
		Spec: appsv1beta1.StatefulSetSpec{
			Replicas: new(int32),
		},
	}
	testDaemonSet := &extensionsv1beta1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-daemonset",
			Namespace: "test-namespace",
		},
		Spec: extensionsv1beta1.DaemonSetSpec{
			RevisionHistoryLimit: new(int32),
		},
	}
	testIngress := &extensionsv1beta1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "test-namespace",
		},
		Spec: extensionsv1beta1.IngressSpec{
			Rules: []extensionsv1beta1.IngressRule{},
		},
	}
	testHpa := &autoscalingv1.HorizontalPodAutoscaler{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-hpa",
			Namespace: "test-namespace",
		},
		Spec: autoscalingv1.HorizontalPodAutoscalerSpec{
			MinReplicas: new(int32),
		},
	}
	testCm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "test-namespace",
		},
	}
	testSecret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
		},
	}
	testNamespace := cloneNamespace(templateNamespace)
	testSpec := cloneAdmissionReview(templateAdmReview)
	clientset = fake.NewSimpleClientset(testNamespace, testPod, testSvc, testReplicaSet, testDeployment, testStatefulSet, testDaemonSet, testIngress, testHpa, testCm, testSecret)
	req := httptest.NewRequest("POST", "http://localhost:8080/", constructPostBody(testSpec))
	webhookHandler(rw, req)

	admReview := getAdmissionReview(rw)

	assert.False(t, admReview.Status.Allowed, "should reject if the namespace has workload resources")
	assert.Contains(t, admReview.Status.Result.Reason, "The namespace test-namespace you are trying to remove contains one or more of these resources: [pods(1) services(1) replicasets(1) deployments(1) statefulsets(1) daemonsets(1) ingresses(1) horizontalpodautoscalers(1)]. Please delete them and try again.")
}

func TestNonEmptyNamespaceWithIgnoredResourcesWebhookHandler(t *testing.T) {
	rw := httptest.NewRecorder()

	testCm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "test-namespace",
		},
	}
	testSecret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
		},
	}
	testNamespace := cloneNamespace(templateNamespace)
	testSpec := cloneAdmissionReview(templateAdmReview)
	clientset = fake.NewSimpleClientset(testNamespace, testCm, testSecret)
	req := httptest.NewRequest("POST", "http://localhost:8080/", constructPostBody(testSpec))
	webhookHandler(rw, req)

	admReview := getAdmissionReview(rw)

	assert.True(t, admReview.Status.Allowed, "should approve if the namespace has ignored resources")
}

func TestStatusHandler200(t *testing.T) {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://localhost:8080/status.html", nil)
	statusHandler(rw, req)
	assert.Equal(t, http.StatusOK, rw.Code, "/status.html should return 200")
}
