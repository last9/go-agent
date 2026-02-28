package mongodb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSanitizedCommandString_Empty(t *testing.T) {
	assert.Equal(t, "", sanitizedCommandString(nil))
	assert.Equal(t, "", sanitizedCommandString(bson.Raw{}))
}

func TestSanitizedCommandString_NormalCommand(t *testing.T) {
	cmd, err := bson.Marshal(bson.M{"find": "users", "filter": bson.M{"name": "Alice"}})
	assert.NoError(t, err)

	result := sanitizedCommandString(bson.Raw(cmd))
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "find")
	assert.Contains(t, result, "users")
	assert.NotContains(t, result, "...")
}

func TestSanitizedCommandString_Truncation(t *testing.T) {
	// Build a command larger than maxStatementLength (4096 bytes)
	largeValue := strings.Repeat("x", maxStatementLength+1000)
	cmd, err := bson.Marshal(bson.M{"insert": "books", "data": largeValue})
	assert.NoError(t, err)

	result := sanitizedCommandString(bson.Raw(cmd))
	assert.True(t, strings.HasSuffix(result, "..."), "should be truncated with ...")
	assert.LessOrEqual(t, len(result), maxStatementLength+3) // 4096 + "..."
}

func TestSanitizedCommandString_ExactlyAtLimit(t *testing.T) {
	// A small command well under the limit should not be truncated
	cmd, err := bson.Marshal(bson.M{"delete": "items", "deletes": bson.A{bson.M{"q": bson.M{"_id": 1}}}})
	assert.NoError(t, err)

	result := sanitizedCommandString(bson.Raw(cmd))
	assert.NotEmpty(t, result)
	assert.NotContains(t, result, "...")
}
