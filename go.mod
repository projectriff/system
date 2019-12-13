module github.com/projectriff/system

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/google/go-cmp v0.3.1
	github.com/google/go-containerregistry v0.0.0-20191002200252-ff1ac7f97758
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.1
	// equivelent of kubernetes-1.16.3 tag for each k8s.io repo
	k8s.io/api v0.0.0-20191114100352-16d7abae0d2a
	k8s.io/apimachinery v0.0.0-20191028221656-72ed19daf4bb
	k8s.io/client-go v0.0.0-20191114101535-6c5935290e33
	k8s.io/code-generator v0.0.0-20191004115455-8e001e5d1894
	sigs.k8s.io/controller-runtime v0.4.0
)
