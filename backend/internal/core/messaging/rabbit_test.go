package messaging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShardIsStableForPositiveAndNegativeChatIDs(t *testing.T) {
	require.Equal(t, 3, Shard(11, 8))
	require.Equal(t, 5, Shard(-11, 8))
	require.Equal(t, Shard(123456789, 8), Shard(123456789, 8))
}
