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
	Name            string             `json:"name"`
	Namespace       string             `json:"namespace"`
	Owner           string             `json:"owner"`
	Entrances       []Entrance         `json:"entrances,omitempty"`
	SharedEntrances []SharedEntrance   `json:"sharedEntrances,omitempty"`
	Permissions     []AppPermission    `json:"permissions,omitempty"`
	Permission      *AppPermissionSpec `json:"permission,omitempty"`
	Options         *AppOptionsSpec    `json:"options,omitempty"`
	Settings        AppSettings        `json:"settings,omitempty"`
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

type SharedEntrance struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Title     string `json:"title"`
	Icon      string `json:"icon,omitempty"`
	AuthLevel string `json:"authLevel,omitempty"`
	Invisible bool   `json:"invisible,omitempty"`
}

type AppPermission struct {
	Group    string   `json:"group"`
	DataType string   `json:"dataType"`
	Version  string   `json:"version"`
	Ops      []string `json:"ops"`
}

type AppPermissionSpec struct {
	AppData  bool           `json:"appData,omitempty"`
	AppCache bool           `json:"appCache,omitempty"`
	UserData []string       `json:"userData,omitempty"`
	SysData  []SysDataEntry `json:"sysData,omitempty"`
	Provider []ProviderEntry `json:"provider,omitempty"`
}

type SysDataEntry struct {
	DataType string   `json:"dataType"`
	AppName  string   `json:"appName"`
	Svc      string   `json:"svc"`
	Port     int      `json:"port"`
	Group    string   `json:"group"`
	Version  string   `json:"version"`
	Ops      []string `json:"ops"`
}

type ProviderEntry struct {
	AppName      string `json:"appName"`
	ProviderName string `json:"providerName"`
}

type AppOptionsSpec struct {
	Dependencies []AppDependency `json:"dependencies,omitempty"`
}

type AppDependency struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // "system", "application"
	Version string `json:"version"`
}

type AppSettings struct {
	Analytics     bool `json:"analytics,omitempty"`
	Clusterscoped bool `json:"clusterscoped,omitempty"`
}

type ApplicationStatus struct {
	State      string       `json:"state"` // installing, running, stopped, error
	UpdateTime *metav1.Time `json:"updateTime,omitempty"`
}

// ProviderRegistryCRD represents a data provider registration.
type ProviderRegistryCRD struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderRegistryCRDSpec   `json:"spec,omitempty"`
	Status ProviderRegistryCRDStatus `json:"status,omitempty"`
}

type ProviderRegistryCRDSpec struct {
	Group       string `json:"group"`
	Kind        string `json:"kind"`
	DataType    string `json:"dataType"`
	Version     string `json:"version"`
	Deployment  string `json:"deployment"`
	Namespace   string `json:"namespace"`
	Endpoint    string `json:"endpoint"`
	Description string `json:"description,omitempty"`
}

type ProviderRegistryCRDStatus struct {
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
	if in.Spec.SharedEntrances != nil {
		out.Spec.SharedEntrances = make([]SharedEntrance, len(in.Spec.SharedEntrances))
		copy(out.Spec.SharedEntrances, in.Spec.SharedEntrances)
	}
	if in.Spec.Permission != nil {
		permCopy := *in.Spec.Permission
		out.Spec.Permission = &permCopy
		if in.Spec.Permission.SysData != nil {
			out.Spec.Permission.SysData = make([]SysDataEntry, len(in.Spec.Permission.SysData))
			copy(out.Spec.Permission.SysData, in.Spec.Permission.SysData)
		}
		if in.Spec.Permission.Provider != nil {
			out.Spec.Permission.Provider = make([]ProviderEntry, len(in.Spec.Permission.Provider))
			copy(out.Spec.Permission.Provider, in.Spec.Permission.Provider)
		}
	}
	if in.Spec.Options != nil {
		optsCopy := *in.Spec.Options
		out.Spec.Options = &optsCopy
		if in.Spec.Options.Dependencies != nil {
			out.Spec.Options.Dependencies = make([]AppDependency, len(in.Spec.Options.Dependencies))
			copy(out.Spec.Options.Dependencies, in.Spec.Options.Dependencies)
		}
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
