package v1alpha1

import "testing"

func TestStreamDefaults(t *testing.T) {
	expectedContentType := "application/octet-stream"
	stream := Stream{}

	stream.SetDefaults(nil)

	actualContentType := stream.Spec.ContentType
	if actualContentType != expectedContentType {
		t.Errorf("expected default stream content-type to be %s, got %s", expectedContentType, actualContentType)
	}
}

func TestStreamDefaultsDoNotOverride(t *testing.T) {
	expectedContentType := "application/x-doom"
	stream := Stream{
		Spec: StreamSpec{
			ContentType: expectedContentType,
		},
	}

	stream.SetDefaults(nil)

	actualContentType := stream.Spec.ContentType
	if actualContentType != expectedContentType {
		t.Errorf("expected stream content-type to be %s, got %s", expectedContentType, actualContentType)
	}
}
