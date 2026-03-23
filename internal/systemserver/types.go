package systemserver

import (
	"github.com/packalares/packalares/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Application CRD types for watching app registrations.

var (
	AppGroup   = "app." + config.APIGroup()
	AppVersion = "v1alpha1"
)

var (
	ApplicationGVR = schema.GroupVersionResource{
		Group:    AppGroup,
		Version:  AppVersion,
		Resource: "applications",
	}
)

// Application represents an installed application.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec,omitempty"`
	Status ApplicationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Application `json:"items"`
}

type ApplicationSpec struct {
	Name        string      `json:"name"`
	Namespace   string      `json:"namespace"`
	Owner       string      `json:"owner"`
	Entrances   []Entrance  `json:"entrances,omitempty"`
	Permissions []AppPermission `json:"permissions,omitempty"`
	Settings    AppSettings `json:"settings,omitempty"`
}

type Entrance struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Title      string `json:"title"`
	Icon       string `json:"icon"`
	AuthLevel  string `json:"authLevel,omitempty"` // "public", "one_factor", "two_factor"
	Invisible  bool   `json:"invisible,omitempty"`
}

type AppPermission struct {
	Group    string   `json:"group"`
	DataType string   `json:"dataType"`
	Version  string   `json:"version"`
	Ops      []string `json:"ops"`
}

type AppSettings struct {
	Analytics   bool `json:"analytics,omitempty"`
	Clusterscoped bool `json:"clusterscoped,omitempty"`
}

type ApplicationStatus struct {
	State      string       `json:"state"` // installing, running, stopped, error
	UpdateTime *metav1.Time `json:"updateTime,omitempty"`
}

// ProviderRegistry represents a data provider registration.
type ProviderRegistry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderRegistrySpec   `json:"spec,omitempty"`
	Status ProviderRegistryStatus `json:"status,omitempty"`
}

type ProviderRegistrySpec struct {
	Group       string `json:"group"`
	Kind        string `json:"kind"`
	DataType    string `json:"dataType"`
	Version     string `json:"version"`
	Deployment  string `json:"deployment"`
	Namespace   string `json:"namespace"`
	Endpoint    string `json:"endpoint"`
	Description string `json:"description,omitempty"`
}

type ProviderRegistryStatus struct {
	State string `json:"state"`
}

// DeepCopy implementations

func (in *Application) DeepCopyObject() runtime.Object {
	out := new(Application)
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec.Entrances != nil {
		out.Spec.Entrances = make([]Entrance, len(in.Spec.Entrances))
		copy(out.Spec.Entrances, in.Spec.Entrances)
	}
	return out
}

func (in *ApplicationList) DeepCopyObject() runtime.Object {
	out := new(ApplicationList)
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Application, len(in.Items))
		for i := range in.Items {
			out.Items[i] = *in.Items[i].DeepCopyObject().(*Application)
		}
	}
	return out
}
