module github.com/projectriff/system

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/google/go-cmp v0.3.1
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.1
	// equivelent of kubernetes-1.15.4 tag for each k8s.io repo
	k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/client-go v0.0.0-20190918200256-06eb1244587a
	k8s.io/code-generator v0.0.0-20190612205613-18da4a14b22b
	sigs.k8s.io/controller-runtime v0.3.0
)
