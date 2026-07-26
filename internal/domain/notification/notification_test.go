package notification

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestValidateTag_PlainUUID(t *testing.T) {
	id := uuid.NewString()
	assert.NoError(t, validateTag(id))
}

func TestValidateTag_WithSuffix(t *testing.T) {
	id := uuid.NewString()
	tag := id + ".progress"
	assert.NoError(t, validateTag(tag))
}

func TestValidateTag_WithNumericSuffix(t *testing.T) {
	id := uuid.NewString()
	tag := id + ".1"
	assert.NoError(t, validateTag(tag))
}

func TestValidateTag_WithMultiPartSuffix(t *testing.T) {
	id := uuid.NewString()
	tag := id + ".a.b.c"
	assert.NoError(t, validateTag(tag))
}

func TestValidateTag_Invalid(t *testing.T) {
	assert.Error(t, validateTag("not-a-uuid"))
}

func TestValidateTag_InvalidUUIDWithSuffix(t *testing.T) {
	assert.Error(t, validateTag("not-a-uuid.suffix"))
}

func TestValidateTag_EmptyString(t *testing.T) {
	assert.Error(t, validateTag(""))
}

func TestFactoryNew_PlainUUID(t *testing.T) {
	f := &Factory{}
	id := uuid.NewString()
	n, err := f.New(id, "test-channel", "Test Title")
	assert.NoError(t, err)
	assert.Equal(t, id, n.Tag())
	assert.Equal(t, "test-channel", n.Channel())
	assert.Equal(t, "Test Title", n.Title())
}

func TestFactoryNew_WithSuffix(t *testing.T) {
	f := &Factory{}
	id := uuid.NewString()
	tag := id + ".progress"
	n, err := f.New(tag, "test-channel", "Test Title")
	assert.NoError(t, err)
	assert.Equal(t, tag, n.Tag())
	assert.Equal(t, "test-channel", n.Channel())
	assert.Equal(t, "Test Title", n.Title())
}

func TestFactoryNew_InvalidTag(t *testing.T) {
	f := &Factory{}
	_, err := f.New("not-a-uuid", "test-channel", "Test Title")
	assert.Error(t, err)
}

func TestFactoryNew_EmptyTag(t *testing.T) {
	f := &Factory{}
	_, err := f.New("", "test-channel", "Test Title")
	assert.Error(t, err)
}
