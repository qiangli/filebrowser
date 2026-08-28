// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !fb_redis

package fbhttp

import "errors"

// ErrRedisCacheUnavailable is returned when a redis upload cache is requested
// from a build that did not compile the redis client in.
//
// It is an explicit refusal rather than a silent fallback to the in-memory
// cache: on a multi-replica deployment the memory cache is WRONG (each replica
// would keep its own view of an in-flight upload), so quietly substituting it
// would turn a configuration error into intermittent corrupt uploads.
var ErrRedisCacheUnavailable = errors.New(
	"filebrowser: this build has no redis upload cache; rebuild with -tags fb_redis")

func newRedisUploadCache(string) (UploadCache, error) { return nil, ErrRedisCacheUnavailable }
