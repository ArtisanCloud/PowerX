// Package redisnamespace defines the shared key and channel scope of a PowerX installation.
package redisnamespace

// Key preserves existing Redis Cluster hash tags. The empty namespace is reserved
// for explicit low-level callers; installed Core instances resolve their scope at startup.
func Key(namespace, key string) string {
	if namespace == "" {
		return key
	}
	return "powerx:" + namespace + ":" + key
}
