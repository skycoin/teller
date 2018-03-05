package dbutil

import (
	"encoding/json"
	"fmt"

	"github.com/boltdb/bolt"
)

// CreateBucketFailedErr is returned if creating a bolt.DB bucket fails
type CreateBucketFailedErr struct {
	Bucket string
	Err    error
}

func (e CreateBucketFailedErr) Error() string {
	return fmt.Sprintf("Create bucket \"%s\" failed: %v", e.Bucket, e.Err)
}

// NewCreateBucketFailedErr returns an CreateBucketFailedErr
func NewCreateBucketFailedErr(bucket []byte, err error) error {
	return CreateBucketFailedErr{
		Bucket: string(bucket),
		Err:    err,
	}
}

// BucketNotExistErr is returned if a bolt.DB bucket does not exist
type BucketNotExistErr struct {
	Bucket string
}

func (e BucketNotExistErr) Error() string {
	return fmt.Sprintf("Bucket \"%s\" doesn't exist", e.Bucket)
}

// NewBucketNotExistErr returns an BucketNotExistErr
func NewBucketNotExistErr(bucket []byte) error {
	return BucketNotExistErr{
		Bucket: string(bucket),
	}
}

// ObjectNotExistErr is returned if an object specified by "key" is not found in a bolt.DB bucket
type ObjectNotExistErr struct {
	Bucket string
	Key    string
}

func (e ObjectNotExistErr) Error() string {
	return fmt.Sprintf("Object with key \"%s\" not found in bucket \"%s\"", e.Key, e.Bucket)
}

// NewObjectNotExistErr returns an ObjectNotExistErr
func NewObjectNotExistErr(bucket, key []byte) error {
	return ObjectNotExistErr{
		Bucket: string(bucket),
		Key:    string(key),
	}
}

// GetBucketObject returns a JSON value from a bucket, unmarshaled to an object.
func GetBucketObject(tx *bolt.Tx, bktName []byte, key string, obj interface{}) error {
	v, err := getBucketValue(tx, bktName, key)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(v, obj); err != nil {
		return fmt.Errorf("decode value failed: %v", err)
	}

	return nil
}

// GetBucketString returns a string value from a bolt.DB bucket
func GetBucketString(tx *bolt.Tx, bktName []byte, key string) (string, error) {
	v, err := getBucketValue(tx, bktName, key)
	if err != nil {
		return "", err
	}

	return string(v), nil
}

func getBucketValue(tx *bolt.Tx, bktName []byte, key string) ([]byte, error) {
	bkt := tx.Bucket(bktName)
	if bkt == nil {
		return nil, NewBucketNotExistErr(bktName)
	}

	bkey := []byte(key)

	v := bkt.Get(bkey)
	if v == nil {
		return nil, NewObjectNotExistErr(bktName, bkey)
	}

	return v, nil
}

// PutBucketValue puts a value into a bucket under key. If the value's type is
// a string, it stores the value as a string. Otherwise, it marshals the value
// to JSON and stores the JSON string.
func PutBucketValue(tx *bolt.Tx, bktName []byte, key string, obj interface{}) error {
	bkt := tx.Bucket(bktName)
	if bkt == nil {
		return NewBucketNotExistErr(bktName)
	}

	bkey := []byte(key)

	switch obj.(type) {
	case []byte:
		return bkt.Put(bkey, obj.([]byte))
	case string:
		return bkt.Put(bkey, []byte(obj.(string)))
	default:
		v, err := json.Marshal(obj)
		if err != nil {
			return fmt.Errorf("encode value failed: %v", err)
		}
		return bkt.Put(bkey, v)
	}
}

// BucketHasKey returns true if a bucket has a non-nil value for a key
func BucketHasKey(tx *bolt.Tx, bktName []byte, key string) (bool, error) {
	bkt := tx.Bucket(bktName)
	if bkt == nil {
		return false, NewBucketNotExistErr(bktName)
	}

	v := bkt.Get([]byte(key))
	return v != nil, nil
}

// NextSequence returns the NextSequence() from the bucket
func NextSequence(tx *bolt.Tx, bktName []byte) (uint64, error) {
	bkt := tx.Bucket(bktName)
	if bkt == nil {
		return 0, NewBucketNotExistErr(bktName)
	}

	return bkt.NextSequence()
}

// ForEach calls ForEach on the bucket
func ForEach(tx *bolt.Tx, bktName []byte, f func(k, v []byte) error) error {
	bkt := tx.Bucket(bktName)
	if bkt == nil {
		return NewBucketNotExistErr(bktName)
	}

	return bkt.ForEach(f)
}
