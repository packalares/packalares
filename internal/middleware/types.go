package middleware

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// MiddlewareRequest CRD types - compatible with Olares marketplace apps.
// Group: apr.bytetrade.io, Version: v1alpha1

const (
	Group   = "apr.bytetrade.io"
	Version = "v1alpha1"
)

var (
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme        = SchemeBuilder.AddToScheme

	MiddlewareRequestGVR = schema.GroupVersionResource{
		Group:    Group,
		Version:  Version,
		Resource: "middlewarerequests",
	}
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&MiddlewareRequest{},
		&MiddlewareRequestList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

type MiddlewareType string

const (
	TypePostgreSQL    MiddlewareType = "postgres"
	TypeRedis         MiddlewareType = "redis"
	TypeNats          MiddlewareType = "nats"
	TypeMongoDB       MiddlewareType = "mongodb"
	TypeMinio         MiddlewareType = "minio"
	TypeRabbitMQ      MiddlewareType = "rabbitmq"
	TypeElasticsearch MiddlewareType = "elasticsearch"
	TypeMariaDB       MiddlewareType = "mariadb"
	TypeMysql         MiddlewareType = "mysql"
	TypeClickHouse    MiddlewareType = "clickhouse"
)

// MiddlewareRequest is the CRD that apps use to request middleware resources.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type MiddlewareRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MiddlewareSpec   `json:"spec,omitempty"`
	Status MiddlewareStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type MiddlewareRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []MiddlewareRequest `json:"items"`
}

type MiddlewareSpec struct {
	App          string         `json:"app"`
	AppNamespace string         `json:"appNamespace"`
	Middleware   MiddlewareType `json:"middleware"`

	Redis      *RedisSpec      `json:"redis,omitempty"`
	PostgreSQL *PostgreSQLSpec `json:"postgreSQL,omitempty"`
	Nats       *NatsSpec       `json:"nats,omitempty"`
}

type MiddlewareStatus struct {
	State      string       `json:"state"`
	Message    string       `json:"message,omitempty"`
	UpdateTime *metav1.Time `json:"updateTime,omitempty"`
}

type RedisSpec struct {
	Password  PasswordVar `json:"password,omitempty"`
	Namespace string      `json:"namespace"`
}

type PostgreSQLSpec struct {
	User      string        `json:"user"`
	Password  PasswordVar   `json:"password,omitempty"`
	Databases []PGDatabase  `json:"databases"`
}

type PGDatabase struct {
	Name        string   `json:"name"`
	Distributed bool     `json:"distributed,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
}

type NatsSpec struct {
	User     string      `json:"user"`
	Password PasswordVar `json:"password,omitempty"`
	Subjects []Subject   `json:"subjects,omitempty"`
}

type Subject struct {
	Name       string     `json:"name"`
	Permission Permission `json:"permission"`
}

type Permission struct {
	Pub string `json:"pub"`
	Sub string `json:"sub"`
}

type PasswordVar struct {
	Value     string          `json:"value,omitempty"`
	ValueFrom *PasswordSource `json:"valueFrom,omitempty"`
}

type PasswordSource struct {
	SecretKeyRef *SecretKeyRef `json:"secretKeyRef,omitempty"`
}

type SecretKeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// DeepCopyObject implements runtime.Object.
func (in *MiddlewareRequest) DeepCopyObject() runtime.Object {
	out := new(MiddlewareRequest)
	in.DeepCopyInto(out)
	return out
}

func (in *MiddlewareRequest) DeepCopyInto(out *MiddlewareRequest) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

func (in *MiddlewareRequestList) DeepCopyObject() runtime.Object {
	out := new(MiddlewareRequestList)
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]MiddlewareRequest, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}
