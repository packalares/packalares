package api

// CRDManifestYAML returns the raw YAML for the User CRD that can be applied
// via kubectl. This is an alternative to the programmatic registration in crd.go.
const CRDManifestYAML = `---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: users.iam.kubesphere.io
spec:
  group: iam.kubesphere.io
  names:
    plural: users
    singular: user
    kind: User
    listKind: UserList
    categories:
    - iam
  scope: Cluster
  versions:
  - name: v1alpha2
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              email:
                type: string
              initialPassword:
                type: string
              lang:
                type: string
              description:
                type: string
              displayName:
                type: string
              groups:
                type: array
                items:
                  type: string
            required:
            - email
          status:
            type: object
            properties:
              state:
                type: string
              reason:
                type: string
              lastTransitionTime:
                type: string
                format: date-time
              lastLoginTime:
                type: string
                format: date-time
    subresources:
      status: {}
    additionalPrinterColumns:
    - name: Email
      type: string
      jsonPath: .spec.email
    - name: Status
      type: string
      jsonPath: .status.state
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: globalroles.iam.kubesphere.io
spec:
  group: iam.kubesphere.io
  names:
    plural: globalroles
    singular: globalrole
    kind: GlobalRole
    listKind: GlobalRoleList
    categories:
    - iam
  scope: Cluster
  versions:
  - name: v1alpha2
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        x-kubernetes-preserve-unknown-fields: true
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: globalrolebindings.iam.kubesphere.io
spec:
  group: iam.kubesphere.io
  names:
    plural: globalrolebindings
    singular: globalrolebinding
    kind: GlobalRoleBinding
    listKind: GlobalRoleBindingList
    categories:
    - iam
  scope: Cluster
  versions:
  - name: v1alpha2
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        x-kubernetes-preserve-unknown-fields: true
`
