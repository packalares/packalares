package kubesphere

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SchemeGroupVersion is the group version used to register KubeSphere IAM objects.
// This is kept compatible with the original KubeSphere CRD group so that
// BFL, app-service, and other components that reference iam.kubesphere.io work
// without modification.
var SchemeGroupVersion = schema.GroupVersion{Group: "iam.kubesphere.io", Version: "v1alpha2"}

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&User{},
		&UserList{},
		&GlobalRole{},
		&GlobalRoleList{},
		&GlobalRoleBinding{},
		&GlobalRoleBindingList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// ---------------------------------------------------------------------------
// User CRD — compatible with iam.kubesphere.io/v1alpha2
// ---------------------------------------------------------------------------

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories="iam",scope="Cluster"
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              UserSpec   `json:"spec"`
	Status            UserStatus `json:"status,omitempty"`
}

type UserSpec struct {
	Email           string   `json:"email"`
	InitialPassword string   `json:"initialPassword,omitempty"`
	Lang            string   `json:"lang,omitempty"`
	Description     string   `json:"description,omitempty"`
	DisplayName     string   `json:"displayName,omitempty"`
	Groups          []string `json:"groups,omitempty"`
}

type UserState string

const (
	UserActive            UserState = "Active"
	UserDisabled          UserState = "Disabled"
	UserAuthLimitExceeded UserState = "AuthLimitExceeded"
)

type UserStatus struct {
	State              UserState    `json:"state,omitempty"`
	Reason             string       `json:"reason,omitempty"`
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	LastLoginTime      *metav1.Time `json:"lastLoginTime,omitempty"`
}

// +kubebuilder:object:root=true
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}

// ---------------------------------------------------------------------------
// GlobalRole
// ---------------------------------------------------------------------------

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories="iam",scope="Cluster"
type GlobalRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Rules             []rbacv1.PolicyRule `json:"rules" protobuf:"bytes,2,rep,name=rules"`
}

// +kubebuilder:object:root=true
type GlobalRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalRole `json:"items"`
}

// ---------------------------------------------------------------------------
// GlobalRoleBinding
// ---------------------------------------------------------------------------

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories="iam",scope="Cluster"
type GlobalRoleBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Subjects          []rbacv1.Subject `json:"subjects,omitempty" protobuf:"bytes,2,rep,name=subjects"`
	RoleRef           rbacv1.RoleRef   `json:"roleRef" protobuf:"bytes,3,opt,name=roleRef"`
}

// +kubebuilder:object:root=true
type GlobalRoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalRoleBinding `json:"items"`
}

// DeepCopyObject implementations for runtime.Object interface

func (in *User) DeepCopyObject() runtime.Object {
	out := new(User)
	in.DeepCopyInto(out)
	return out
}

func (in *User) DeepCopyInto(out *User) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	if in.Spec.Groups != nil {
		out.Spec.Groups = make([]string, len(in.Spec.Groups))
		copy(out.Spec.Groups, in.Spec.Groups)
	}
	out.Status = in.Status
	if in.Status.LastTransitionTime != nil {
		t := *in.Status.LastTransitionTime
		out.Status.LastTransitionTime = &t
	}
	if in.Status.LastLoginTime != nil {
		t := *in.Status.LastLoginTime
		out.Status.LastLoginTime = &t
	}
}

func (in *UserList) DeepCopyObject() runtime.Object {
	out := new(UserList)
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]User, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

func (in *GlobalRole) DeepCopyObject() runtime.Object {
	out := new(GlobalRole)
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Rules != nil {
		out.Rules = make([]rbacv1.PolicyRule, len(in.Rules))
		copy(out.Rules, in.Rules)
	}
	return out
}

func (in *GlobalRoleList) DeepCopyObject() runtime.Object {
	out := new(GlobalRoleList)
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]GlobalRole, len(in.Items))
		for i := range in.Items {
			out.Items[i] = in.Items[i]
			in.Items[i].ObjectMeta.DeepCopyInto(&out.Items[i].ObjectMeta)
		}
	}
	return out
}

func (in *GlobalRoleBinding) DeepCopyObject() runtime.Object {
	out := new(GlobalRoleBinding)
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Subjects != nil {
		out.Subjects = make([]rbacv1.Subject, len(in.Subjects))
		copy(out.Subjects, in.Subjects)
	}
	out.RoleRef = in.RoleRef
	return out
}

func (in *GlobalRoleBindingList) DeepCopyObject() runtime.Object {
	out := new(GlobalRoleBindingList)
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]GlobalRoleBinding, len(in.Items))
		for i := range in.Items {
			out.Items[i] = in.Items[i]
			in.Items[i].ObjectMeta.DeepCopyInto(&out.Items[i].ObjectMeta)
		}
	}
	return out
}
