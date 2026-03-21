package kubesphere

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// RegisterCRDs creates the iam.kubesphere.io CRDs in the cluster.
// This replaces KubeSphere's CRD registration.
func RegisterCRDs(config *rest.Config) error {
	client, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("create apiextensions client: %w", err)
	}

	crds := []*apiextensionsv1.CustomResourceDefinition{
		userCRD(),
		globalRoleCRD(),
		globalRoleBindingCRD(),
	}

	ctx := context.Background()
	for _, crd := range crds {
		existing, err := client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
		if err == nil {
			// Update existing
			crd.ResourceVersion = existing.ResourceVersion
			if _, err := client.ApiextensionsV1().CustomResourceDefinitions().Update(ctx, crd, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("update CRD %s: %w", crd.Name, err)
			}
		} else {
			// Create new
			if _, err := client.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create CRD %s: %w", crd.Name, err)
			}
		}
	}

	return nil
}

func userCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "users.iam.kubesphere.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "iam.kubesphere.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "users",
				Singular:   "user",
				Kind:       "User",
				ListKind:   "UserList",
				Categories: []string{"iam"},
			},
			Scope: apiextensionsv1.ClusterScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha2",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"email":           {Type: "string"},
										"initialPassword": {Type: "string"},
										"lang":            {Type: "string"},
										"description":     {Type: "string"},
										"displayName":     {Type: "string"},
										"groups": {
											Type:  "array",
											Items: &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &apiextensionsv1.JSONSchemaProps{Type: "string"}},
										},
									},
									Required: []string{"email"},
								},
								"status": {
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"state":              {Type: "string"},
										"reason":             {Type: "string"},
										"lastTransitionTime": {Type: "string", Format: "date-time"},
										"lastLoginTime":      {Type: "string", Format: "date-time"},
									},
								},
							},
						},
					},
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{Name: "Email", Type: "string", JSONPath: ".spec.email"},
						{Name: "Status", Type: "string", JSONPath: ".status.state"},
					},
				},
			},
		},
	}
}

func globalRoleCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "globalroles.iam.kubesphere.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "iam.kubesphere.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "globalroles",
				Singular:   "globalrole",
				Kind:       "GlobalRole",
				ListKind:   "GlobalRoleList",
				Categories: []string{"iam"},
			},
			Scope: apiextensionsv1.ClusterScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha2",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type:                   "object",
							XPreserveUnknownFields: boolPtr(true),
						},
					},
				},
			},
		},
	}
}

func globalRoleBindingCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "globalrolebindings.iam.kubesphere.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "iam.kubesphere.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "globalrolebindings",
				Singular:   "globalrolebinding",
				Kind:       "GlobalRoleBinding",
				ListKind:   "GlobalRoleBindingList",
				Categories: []string{"iam"},
			},
			Scope: apiextensionsv1.ClusterScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha2",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type:                   "object",
							XPreserveUnknownFields: boolPtr(true),
						},
					},
				},
			},
		},
	}
}

func boolPtr(b bool) *bool { return &b }
