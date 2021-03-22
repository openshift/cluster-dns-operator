// Code generated by go-bindata. DO NOT EDIT.
// sources:
// assets/dns/cluster-role-binding.yaml (223B)
// assets/dns/cluster-role.yaml (397B)
// assets/dns/daemonset.yaml (6.384kB)
// assets/dns/metrics/cluster-role-binding.yaml (279B)
// assets/dns/metrics/cluster-role.yaml (246B)
// assets/dns/metrics/role-binding.yaml (293B)
// assets/dns/metrics/role.yaml (284B)
// assets/dns/namespace.yaml (369B)
// assets/dns/service-account.yaml (85B)
// assets/dns/service.yaml (520B)

package manifests

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func bindataRead(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, gz)
	clErr := gz.Close()

	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}
	if clErr != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type asset struct {
	bytes  []byte
	info   os.FileInfo
	digest [sha256.Size]byte
}

type bindataFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

func (fi bindataFileInfo) Name() string {
	return fi.name
}
func (fi bindataFileInfo) Size() int64 {
	return fi.size
}
func (fi bindataFileInfo) Mode() os.FileMode {
	return fi.mode
}
func (fi bindataFileInfo) ModTime() time.Time {
	return fi.modTime
}
func (fi bindataFileInfo) IsDir() bool {
	return false
}
func (fi bindataFileInfo) Sys() interface{} {
	return nil
}

var _assetsDnsClusterRoleBindingYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x6c\xce\x31\x8e\x83\x40\x0c\x05\xd0\x7e\x4e\xe1\x0b\xc0\x6a\xbb\xd5\x74\x9b\xdc\x80\x48\xe9\xcd\x8c\x09\x0e\x60\xa3\xb1\x87\x22\xa7\x8f\x10\x4a\x45\x3a\x17\xfe\xff\xfd\x89\x25\x47\xb8\xce\xd5\x9c\x4a\xa7\x33\x5d\x58\x32\xcb\x23\xe0\xca\x77\x2a\xc6\x2a\x11\x4a\x8f\xa9\xc5\xea\xa3\x16\x7e\xa1\xb3\x4a\x3b\xfd\x59\xcb\xfa\xb3\xfd\x86\x85\x1c\x33\x3a\xc6\x00\x00\x20\xb8\x50\x04\x5d\x49\x6c\xe4\xc1\x9b\x2c\x16\xac\xf6\x4f\x4a\x6e\x31\x34\x70\x78\x37\x2a\x1b\x27\xfa\x4f\x49\xab\x78\xf8\xc4\xf6\xe7\xe3\xb6\x15\xd3\xa9\xa7\xe8\x4c\x1d\x0d\x3b\x74\x9a\x1d\xbe\xd3\xef\x00\x00\x00\xff\xff\xfa\x62\xe7\x50\xdf\x00\x00\x00")

func assetsDnsClusterRoleBindingYamlBytes() ([]byte, error) {
	return bindataRead(
		_assetsDnsClusterRoleBindingYaml,
		"assets/dns/cluster-role-binding.yaml",
	)
}

func assetsDnsClusterRoleBindingYaml() (*asset, error) {
	bytes, err := assetsDnsClusterRoleBindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "assets/dns/cluster-role-binding.yaml", size: 223, mode: os.FileMode(420), modTime: time.Unix(1, 0)}
	a := &asset{bytes: bytes, info: info, digest: [32]uint8{0xd9, 0xf6, 0x2a, 0x3b, 0x84, 0xd7, 0x3e, 0xc4, 0xe1, 0x70, 0x66, 0x31, 0xda, 0xc4, 0x2f, 0x53, 0x27, 0x29, 0x13, 0xfe, 0x80, 0x36, 0xc5, 0xa1, 0x70, 0xdc, 0x2d, 0xef, 0xcf, 0xe0, 0xc4, 0xeb}}
	return a, nil
}

var _assetsDnsClusterRoleYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x7c\x90\xb1\x4e\xc4\x30\x10\x44\x7b\x7f\x85\x75\xfd\x05\xd1\xa1\xb4\x14\xf4\x14\xf4\x1b\x67\x50\x96\xe4\x76\xad\xdd\x75\x4e\xe2\xeb\x51\x2e\x57\xa0\x8b\xa0\x9b\x19\xd9\xf3\x3c\x9e\x59\xc6\x3e\xbf\x2e\xcd\x03\xf6\xae\x0b\x12\x55\xfe\x80\x39\xab\xf4\xd9\x06\x2a\x1d\xb5\x98\xd4\xf8\x9b\x82\x55\xba\xf9\xc5\x3b\xd6\xa7\xf5\x39\x5d\x10\x34\x52\x50\x9f\x72\x16\xba\xa0\xcf\x5a\x21\x3e\xf1\x67\x9c\x47\xf1\x64\x6d\x81\xf7\xe9\x9c\xa9\xf2\x9b\x69\xab\xbe\x9d\x3c\xe7\xd3\x29\xe5\x6c\x70\x6d\x56\x70\xcf\x20\x63\x55\x96\xf0\x9b\x73\xd8\xca\x05\xbb\xa9\x3a\xee\x62\x63\x78\xa5\x3d\x5f\x61\xc3\xfd\xee\xc2\x1e\x37\x71\xa5\x28\x53\x3a\x02\xb7\x01\x90\xe0\xf2\x7b\xc1\xf1\x0d\xa1\x33\xc4\xb0\x32\xae\x0f\x84\x62\xa0\xc0\x1f\xcd\x8f\x5f\x73\x2c\xf6\x36\x7c\xa1\x04\x95\x02\xf7\xff\x00\x3f\x01\x00\x00\xff\xff\x76\x1b\x55\x2e\x8d\x01\x00\x00")

func assetsDnsClusterRoleYamlBytes() ([]byte, error) {
	return bindataRead(
		_assetsDnsClusterRoleYaml,
		"assets/dns/cluster-role.yaml",
	)
}

func assetsDnsClusterRoleYaml() (*asset, error) {
	bytes, err := assetsDnsClusterRoleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "assets/dns/cluster-role.yaml", size: 397, mode: os.FileMode(420), modTime: time.Unix(1, 0)}
	a := &asset{bytes: bytes, info: info, digest: [32]uint8{0x84, 0xae, 0xd1, 0xba, 0xfa, 0x6b, 0xf8, 0x6e, 0x8d, 0x28, 0xc2, 0xa7, 0xaf, 0xc9, 0x3b, 0xc7, 0xcd, 0x80, 0xbe, 0xec, 0x98, 0xb4, 0x61, 0xa0, 0x9, 0xae, 0xa, 0xd8, 0xb2, 0x2e, 0x16, 0xf2}}
	return a, nil
}

var _assetsDnsDaemonsetYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xcc\x59\x6d\x57\x1b\xb7\xf2\x7f\xcf\xa7\x98\x2e\xfc\x43\xd2\xb0\x60\x27\x21\xcd\x7f\x13\x7a\xeb\x82\x29\x9c\x06\xf0\xc1\x4e\xf3\x82\xc3\xe5\xc8\xda\xb1\x57\xd7\x5a\x49\x95\xb4\x6b\xf6\x80\xbf\xfb\x3d\xd2\xfa\x61\xd7\x6b\x68\xd3\xfb\x70\x6e\x5e\x38\xb6\x66\xe6\xa7\x99\xd1\x3c\x49\x4c\x98\x88\x23\x38\x21\x98\x4a\xd1\x47\xbb\x45\x14\xfb\x0d\xb5\x61\x52\x44\x40\x94\x32\x07\x79\x7b\x6b\x1b\x04\x49\x71\xcf\x7f\x1a\x45\x28\x02\x11\x31\x70\x32\x44\x6e\x80\x68\x04\x83\x16\x88\x05\x9d\x09\xcb\x52\xdc\x32\x0a\x69\xb4\x05\x60\x31\x55\x9c\x58\x74\xdf\x01\x16\xab\xfe\x3b\xea\x9c\x51\xec\x50\x2a\x33\x61\x2f\x49\x8a\x11\xc4\xc2\xcc\xa9\x4a\x33\xa9\x99\x2d\x8e\x39\x31\xa6\x24\x9a\xc2\x58\x4c\x43\x21\x63\x0c\xa9\x66\x96\x51\xc2\xe7\xdc\x54\x0a\x4b\x98\x40\x6d\x16\xe8\xa1\xd7\xb4\x8a\x08\xb0\x0d\x2c\x25\x63\x04\x66\xd6\xb5\x5d\x70\x78\x7a\x2f\xe3\xbc\x27\x39\xa3\x45\x04\xe7\xa3\x4b\x69\x7b\x1a\x0d\x0a\xbb\xe4\xb2\xa8\x53\x26\x88\x65\x52\x5c\xa0\x31\x4e\x64\xce\x7e\x4a\x38\x1f\x12\x3a\x19\xc8\xcf\x72\x6c\xae\x44\x57\x6b\xa9\x97\x72\x54\xa6\x29\x71\xae\xbe\x81\x80\x4a\x8d\xb1\x30\x01\xdc\x2e\xc9\x44\x8f\x8d\xa7\x85\x54\x8a\x51\xb0\x07\xc1\x01\x5a\x7a\x30\xe7\x3c\x38\x96\x1a\x47\x8c\x63\x55\x24\x97\x3c\x4b\xf1\xc2\x39\x70\x69\xf9\xca\x76\x07\xc3\xc6\x61\xc9\xb4\xa4\x02\xa4\x8e\xbf\x47\x6c\x12\x41\x75\x87\x0a\x87\x46\x12\x5f\x09\x5e\x44\x60\x75\xb6\x12\x55\x52\xd7\xf7\x59\xfa\xbd\x27\xb5\x8d\xe0\xf0\xed\xe1\xdb\x0a\x4a\xf3\x04\xdc\xb9\x4a\x2b\xa9\xe4\x11\x7c\x39\xe9\x7d\x3b\x52\x68\xa9\xda\x88\x36\x38\x5e\xa1\x39\xed\x99\x40\x63\x7a\x5a\x0e\x31\xaa\xf0\x27\xd6\xaa\x5f\xd0\x56\x97\x00\x54\xe9\x09\x27\x55\xd4\x09\x5e\x95\x0f\xed\x0f\xed\xda\xb2\xa1\x09\x3a\x75\xce\x06\x83\x5e\x85\xc0\x04\xb3\x8c\xf0\x13\xe4\xa4\xe8\x23\x95\x22\x36\x11\xb4\x5b\x55\x6d\x51\x33\x19\x2f\x69\x55\x03\x4d\x46\x29\x1a\x33\x48\x34\x9a\x44\xf2\x38\x82\xea\x9e\x23\xc2\x78\xa6\xb1\x42\xad\xca\xba\x08\x96\x99\xdd\x80\xcb\x59\x8e\xdf\xec\x87\x04\x09\xb7\xc9\x26\x47\xb4\x3e\xb4\xfe\xb2\x23\xde\xb7\x9e\xd1\xf8\xf0\x5f\xf0\xc4\x61\xe5\xd8\x8d\xcc\x34\x45\x13\xd5\x22\xf9\xf7\x0c\x8d\x35\x75\x53\xa9\xca\x22\x38\x6c\xa5\xb5\xc5\x14\x53\xa9\x8b\x08\x7e\x68\x5d\xb0\xb5\x2a\x32\xc9\x86\x18\xea\x21\xa1\xa1\xd2\xf2\xbe\xf8\x86\x8a\xe2\x93\xba\x12\xe7\x61\xc8\xe5\xd8\x4a\x63\x63\xd4\xba\xb6\x6e\x90\x66\x1a\x43\xce\x8c\x45\x11\x92\x38\xd6\x68\xcc\x51\xf4\xff\xed\xc3\x77\x35\x3e\xcb\x4d\x48\x99\x4a\x50\x87\x26\x63\x16\xcd\xd1\xe0\x73\xff\xae\x7b\x7c\x72\xd6\xbd\xbb\xee\x77\xee\xbe\x9e\x0f\xce\xee\x3a\xdd\xfe\x5d\xfb\xcd\x87\xbb\x5f\x8e\x2f\xee\xfa\x67\x9d\x37\x87\xef\xf7\x56\x5c\xdd\xe3\x93\x3f\xe0\x6b\xe0\x1c\xff\x7c\xfc\xa7\x70\x36\xf2\x3d\x83\x56\xb3\x2c\x53\xc6\x6a\x24\xe9\x91\x0b\xcf\xe8\xe0\xa0\xfd\xe6\x87\xfd\xd6\x7e\x6b\xbf\xed\x9c\xf0\xf6\xa0\xe9\x05\xd4\x36\x74\x25\xf1\xc8\x97\x31\xcb\xcd\x81\xd2\x2c\x27\x16\xdd\xf7\x7d\xaa\x6d\x43\x64\x4e\x0f\x27\x58\x3c\x23\x39\xc1\xe2\x4f\xd7\xbc\xda\xf9\x2c\x2a\x55\x8a\x56\x33\x6a\xfe\x72\x68\xb6\x9f\x08\xcd\x77\xab\xd0\x7c\xba\xf8\xaf\x97\xf7\x8a\x75\x4f\x29\xea\x7c\xf3\x47\xe5\xbf\xd2\x51\xcb\x1e\xec\x8c\xe2\x39\xea\xff\x99\xfe\xea\x33\xc8\xcd\x0c\x52\x58\xbc\xaf\x55\x37\x67\x3f\xe3\x38\xc6\x78\xad\xa5\x3d\xdf\x41\x13\x69\xac\xf1\x81\xf2\x4c\xfb\xf4\x4c\x15\x27\xa0\xc8\xe1\xb2\x73\xd1\xed\x77\xaf\x7f\xeb\x5e\xfb\x39\xe9\xf8\xf3\x97\xfe\xa0\x7b\x7d\x77\x72\x75\xd1\x39\xbf\xdc\x34\x2f\x2d\xc4\x51\xe4\x4d\x35\x1c\xd2\xf9\x71\xb7\x5f\x51\x62\x1b\x8e\xdd\x34\x01\x52\x43\x39\x8e\x19\x54\x44\x13\x8b\x31\xb8\x0a\x02\x72\xb4\x18\xb0\x4c\x4d\xea\xf2\x6a\xd0\x8d\xe0\x54\x6a\x10\x72\xba\x07\x28\x4c\xa6\x11\x6c\x82\x06\xbd\x5a\x1a\x39\xb1\x2c\xc7\x72\xd0\xfb\x08\x23\xa9\x01\x09\x4d\xea\x84\xbd\x1a\x26\x11\x40\x38\x23\x06\xa6\xcc\x26\x0e\x6b\xdd\x5e\x93\x8d\x46\xec\x1e\xa6\x8c\x73\x20\xdc\x48\x18\x22\x90\x38\xc6\x78\xbf\x82\x93\x13\x9e\x61\x04\x81\x8f\x91\x50\xe3\x98\x19\xab\x8b\x7d\xa9\x50\x98\x84\x8d\x6c\xb8\x46\x30\x39\x0d\x1a\xa3\x55\xc5\x75\x07\x43\x26\x0e\x86\xc4\x24\xd5\x22\x40\x2b\x3f\x1e\xab\x46\x7c\xd7\x64\x07\x7f\x46\x61\x26\x41\x31\x85\xae\xf3\x6c\x55\x7b\x98\x26\x0a\x76\xff\x21\x87\x06\x42\x05\x8f\x70\xef\x2a\x3d\x4c\x9c\x89\x8f\x8f\x3e\xc6\x3e\xc2\x94\x30\xfb\x11\xf0\x9e\x59\x68\xed\xc2\xa0\x7b\x7d\x51\x45\xb8\xea\x75\x2f\xfb\x67\xe7\xa7\x83\xbb\x8b\xce\xf5\xaf\xdd\xeb\xa3\x60\x65\xeb\x18\x05\xfa\xd3\xac\xa7\x5a\x50\x11\x3f\xbb\xea\x0f\xfa\x77\xa7\xe7\x9f\xbb\x47\xc1\x2a\x0e\xab\x1c\x83\xee\x45\xaf\xc1\xb0\x6f\x53\x15\x54\xd5\x38\x3f\xed\x1f\xed\xee\xc1\xae\xcf\x7a\x08\x35\x84\x64\x19\x3a\xf0\xe9\xd3\x27\x08\x76\x1e\x16\x01\x38\xab\x49\x6e\xc3\x05\x99\x20\x10\x3f\xe4\x4b\x4d\x74\x01\x2e\x55\x56\x61\x20\x79\x5c\xa6\x90\x5f\xdf\x35\x40\xac\xd5\x6c\x98\x59\x34\xd5\x93\xa7\x0a\xc2\x11\x84\xe1\x8a\x1a\x4a\xc1\x0b\xb7\xf1\xca\xc8\x59\xe0\x7e\x2f\x4d\xaa\x6b\x32\x4d\xdc\xbe\xa5\xd3\x63\x59\x2b\x9d\x31\x52\xee\x02\x3b\xec\x80\xc9\xe9\x1d\x53\xa6\x46\x76\xf1\x6d\x72\x0a\x4c\x38\xf8\x85\xdd\x37\x3f\xdd\xce\x82\x06\x94\xb3\xf8\x14\x2d\x4d\x16\xfe\x81\xf3\x1e\x8c\xb4\x4c\x81\xf2\xcc\x58\xd4\xae\x36\x02\x1b\x81\x2a\x0b\xda\x3e\x7c\x45\x48\x9d\x8b\x0c\xe6\xa8\x09\x07\xab\x19\x9a\x06\xa6\x95\x10\x4b\x60\x36\x82\xf3\x5e\xfe\x6e\xcf\x7d\xbe\xf7\x9f\xef\x40\xe6\xa8\xdd\x6c\xeb\xab\x88\x5b\x5f\xae\xec\xc3\x20\x41\xb0\x53\x09\x9c\xb8\x7c\x17\x1b\x80\x9d\xdd\xce\xc0\x18\x15\x97\x45\x8a\xc2\xce\x73\xf4\xd7\x4c\x17\x1a\xa4\x70\x27\x84\x1a\xae\x14\x8a\xbe\x25\x74\x02\x2f\xaf\xfa\xbd\xf6\xdb\x57\x10\x82\x4d\xa4\x41\xa7\x97\x90\xb6\x01\x6c\x32\xe5\xfa\xa2\x9b\xe1\x81\x4b\x12\x0f\x09\x27\x82\xa2\x36\x5e\x4f\xd7\xd8\x98\xaf\x25\x84\x26\x4c\x8c\xe1\xe4\xb2\x0f\x36\xd1\x32\x1b\x27\x5e\xf5\x35\x3c\x9a\xc6\xe6\xe8\xe5\x6e\xcc\xc6\x10\x5a\xe8\xc0\x4f\xc1\xce\xc3\xaa\x80\xce\x02\x78\x6d\x12\xb7\x9b\x3b\xa0\x9c\xce\xf6\x77\x1e\xea\xf5\x65\x16\x3c\x8e\x35\x2a\x08\x73\x08\xfe\xfe\x31\xd8\x5d\x83\x2f\xff\x2d\xe1\x3b\x9d\xff\xf4\x0e\xf0\xda\x52\x05\xaf\x35\x5a\x5d\x1c\xb5\xfe\x0b\xe6\xfc\x7b\xf7\x7b\xb5\xb6\xa1\x8b\x20\xe6\x12\x64\xe7\xe1\x3b\x77\x54\x37\xdf\xdf\xce\xd6\x58\x1a\x89\x02\xc0\x94\x39\x7a\xb9\xf3\x12\x73\xc2\xdd\xce\x5e\x90\xdd\xce\x82\x57\xeb\xf0\xe0\x32\xe6\xe6\x06\x82\x9d\xbf\x05\x10\xe2\xef\xd0\x82\x17\x2f\x9c\xc8\x36\x53\x65\x22\x42\x28\x10\x5a\x70\x7b\xfb\xd1\x55\x15\xb1\xc1\x1f\xf3\xcc\xbe\x99\x9b\x18\xdc\x1e\x05\x3b\x0f\x0b\xf1\x0d\xfc\x43\x8d\x64\xd2\x58\x1f\xb1\x86\x59\x02\xb7\x1a\x0b\xb5\x95\x6d\xf8\xa2\x62\x62\xb1\x32\x0a\x80\x2f\x5e\x6c\x04\x53\x84\x31\x5a\xd7\xd8\x58\x5c\x29\x19\x66\x0d\xe0\x2b\x96\x9d\x51\x48\x0b\x59\x03\x6c\x9a\xa0\x70\x66\x6b\x3f\x57\xcd\xaf\xea\x4b\x34\x99\x59\x37\x71\x49\x0d\x44\x31\xc8\x04\xc9\x09\xe3\x64\xc8\x38\xb3\xc5\xda\x36\x7d\x4b\x38\x02\x0a\x5f\x83\x80\xca\x8c\xc7\xae\x35\x19\xeb\x8e\xb6\xb2\x21\x1b\xf9\xda\xbd\xd8\x81\x19\x88\x91\xa3\xc5\x78\xab\x79\x66\xa1\x98\x47\x95\xf7\xfe\xf7\xb7\xe1\x2c\x78\xea\x98\xb6\xe1\xe7\x8c\xf1\x18\x08\x08\x9c\x56\xba\x42\x59\x40\xab\x06\xbb\x02\x25\x33\x0d\x34\x33\x56\xa6\x4b\x8d\x47\x8c\x5b\xd4\x18\x3b\x9b\xd7\xb0\x97\xe1\xbb\x0d\x3b\x0f\xeb\x6d\xb5\x6c\x1c\xb5\x46\xf2\xe3\x33\xad\xa4\xd4\xb5\xa3\x14\xfa\x4a\x56\xf6\xdd\x95\x12\xae\x5d\x34\xe7\x2a\x68\x74\x92\xef\x16\x4e\x79\xa2\x93\xcc\xd3\x4a\x95\x79\xb5\x60\x2e\xc3\xf7\x76\xb6\x51\x00\x00\x69\x22\xc1\x47\xf6\xac\x14\x5a\xfc\xd7\xcc\x69\x78\xc2\x15\x3f\x36\x6c\x5f\xdf\xa4\x11\xf4\x9b\xc2\xde\xf9\x68\x70\x75\x72\x15\x6d\x08\x7f\x62\x65\xca\x28\xe1\xbc\x70\x9d\x8d\xe4\x92\xc5\x40\x44\x01\x4c\x50\x29\x8c\xbf\xde\x5a\x18\x62\x42\x72\x56\x19\xde\x17\xa8\xd7\xa8\xb8\x9b\x67\x37\x45\x44\x2a\x63\x36\x62\x18\x43\x5e\x3e\x4f\xba\x28\x14\x88\xf1\x5a\x6c\xba\x8e\xa2\xd6\xcc\x6c\xc4\xc0\xe3\xe3\x7c\xee\x78\x9e\xaf\x69\xf5\x82\xd7\x65\x86\x4b\x59\x8d\xa9\xcc\x31\x5e\xd9\xea\xa3\x9a\x6a\x74\xb7\xc9\x32\x75\x7c\x57\x5c\x4d\x37\x40\xa5\x2a\x80\x26\x99\xae\x27\xc9\x5a\xfd\x31\x1c\x51\xc1\xfb\x16\xbc\xf0\x83\x64\x8d\x96\x09\x37\x9b\x36\x07\x9a\xda\xe1\x7d\xf3\x83\xc8\xe6\x4b\xe7\x9b\xf6\xf2\xd2\x19\x0b\xb3\xb8\x8a\x9d\xe0\x88\x64\x7c\xa1\x95\x9b\x52\xfb\xc8\x91\x5a\xa9\x57\xc8\x93\x6c\x88\x5a\xa0\x1b\xf7\x98\x3c\x90\x26\x02\xce\x44\x76\x5f\x12\xe7\x5c\xe5\x05\xac\xf1\x6e\xbb\xf9\xed\xb2\x5c\xbd\x20\x2a\xaa\xdc\xb7\x2e\x49\xfa\xdc\x9d\x13\x80\x59\x4c\x6b\xf6\x86\x30\xc1\x22\x82\xc5\x8b\xea\x86\x57\xb0\x35\xd2\x33\xf7\x41\xb7\xe4\x2f\x83\x5b\xeb\x18\x1b\x2e\x87\x00\xb6\x50\x18\xc1\x69\x13\x7a\xd3\x4d\x7c\xdb\x5d\x69\x35\xda\x67\x2d\xb4\x92\xbb\xab\x02\x93\x62\x69\xe3\xb6\x9f\xb8\x5c\x66\x18\x17\x96\x3a\x13\xe0\x06\xd0\x62\xea\xda\xc8\x3e\x0c\x4a\x09\x04\xc2\x39\x58\xc2\xc4\x52\xc3\x10\xa4\x72\x24\xa9\x23\xe8\xba\xde\xe0\x08\x65\x4f\xea\x5b\x27\x32\x2e\xca\x3d\x4a\x33\xae\x25\xe7\x4c\x8c\xcb\x12\xe0\xd7\x75\x75\x65\xa5\xce\xa5\xb4\x18\xf9\x81\x35\xf6\x7f\x65\xf0\x8f\x28\x8e\x17\x35\x68\x99\x09\xa7\x67\x82\xa0\x50\x53\x14\xbe\xa3\x65\x6a\x29\xfc\x32\x13\x9c\x4d\xfc\x25\xb5\x32\xc9\x56\x20\xf6\xdc\xfc\xef\xae\xa8\x25\x52\x2c\xa7\xe2\xd5\x62\xc6\x4c\xc9\xfd\x97\x45\x57\xe4\x18\x41\xbb\xf5\x7f\x5b\xff\x0c\x00\x00\xff\xff\x68\x77\xec\xed\xf0\x18\x00\x00")

func assetsDnsDaemonsetYamlBytes() ([]byte, error) {
	return bindataRead(
		_assetsDnsDaemonsetYaml,
		"assets/dns/daemonset.yaml",
	)
}

func assetsDnsDaemonsetYaml() (*asset, error) {
	bytes, err := assetsDnsDaemonsetYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "assets/dns/daemonset.yaml", size: 6384, mode: os.FileMode(420), modTime: time.Unix(1, 0)}
	a := &asset{bytes: bytes, info: info, digest: [32]uint8{0xa2, 0x4d, 0x67, 0xbe, 0xbf, 0xa7, 0x40, 0xda, 0xc1, 0x2f, 0xdb, 0xe, 0xbc, 0x85, 0xc0, 0x8f, 0x57, 0xe1, 0xbc, 0x20, 0xcb, 0x21, 0x4e, 0xe0, 0xe1, 0x31, 0xcf, 0x9c, 0x5b, 0x23, 0x60, 0x11}}
	return a, nil
}

var _assetsDnsMetricsClusterRoleBindingYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x7c\x8f\xb1\x4a\x04\x41\x0c\x86\xfb\x79\x8a\xbc\xc0\xae\xd8\x1d\xd3\xa9\x85\xfd\x09\xf6\xb9\x99\x9c\x1b\x77\x27\x19\x92\xcc\x16\x3e\xbd\x2c\x8a\x08\xe2\xb5\x81\x7c\xdf\xff\xad\x2c\x35\xc3\xd3\x36\x3c\xc8\xce\xba\xd1\x23\x4b\x65\x79\x4b\xd8\xf9\x95\xcc\x59\x25\x83\x5d\xb0\xcc\x38\x62\x51\xe3\x0f\x0c\x56\x99\xd7\x93\xcf\xac\x77\xfb\x7d\x6a\x14\x58\x31\x30\x27\x00\xc1\x46\x19\xaa\xf8\xd4\x54\x38\xd4\x0e\x92\x8f\xcb\x3b\x95\xf0\x9c\x26\xf8\xd2\xbd\x90\xed\x5c\xe8\xa1\x14\x1d\x12\x3f\x7f\xdd\xb4\x51\x2c\x34\x7c\x5a\x4f\xfe\x7d\xf6\x8e\x85\x32\x68\x27\xf1\x85\xaf\xf1\x9b\x6c\xba\xd1\x99\xae\x87\xf9\x4f\xc7\x7f\x6b\x00\xb0\xf3\xb3\xe9\xe8\x37\xba\xd2\x67\x00\x00\x00\xff\xff\x5b\x52\x00\xaa\x17\x01\x00\x00")

func assetsDnsMetricsClusterRoleBindingYamlBytes() ([]byte, error) {
	return bindataRead(
		_assetsDnsMetricsClusterRoleBindingYaml,
		"assets/dns/metrics/cluster-role-binding.yaml",
	)
}

func assetsDnsMetricsClusterRoleBindingYaml() (*asset, error) {
	bytes, err := assetsDnsMetricsClusterRoleBindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "assets/dns/metrics/cluster-role-binding.yaml", size: 279, mode: os.FileMode(420), modTime: time.Unix(1, 0)}
	a := &asset{bytes: bytes, info: info, digest: [32]uint8{0x79, 0x95, 0x6f, 0xa4, 0xd5, 0xed, 0x48, 0x27, 0x41, 0x56, 0x5c, 0xea, 0x5c, 0x89, 0xdc, 0xc1, 0x44, 0x91, 0xd4, 0xb, 0x18, 0x85, 0x79, 0x75, 0xaa, 0x6e, 0xb5, 0x98, 0xbe, 0xc6, 0x33, 0x43}}
	return a, nil
}

var _assetsDnsMetricsClusterRoleYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x34\xcd\x31\x4b\x34\x41\x0c\x87\xf1\x7e\x3e\x45\xe0\xad\x77\x5f\xec\x64\x5a\x05\x3b\x0b\x05\xfb\xec\xce\xdf\xdb\x70\x3b\xc9\x90\x64\x0e\xf4\xd3\x8b\x70\xb6\x0f\x3f\x78\xfe\xd1\xd3\x39\x23\xe1\xe4\x76\x22\x48\x81\x86\x46\xdb\x17\x0d\xb7\x8e\x3c\x30\x83\xd2\x28\x76\xe7\x01\x7a\x7e\x7d\xa7\x8e\x74\xd9\x83\xa0\x6d\x98\x68\x16\x1e\xf2\x01\x0f\x31\xad\xe4\x1b\xef\x2b\xcf\x3c\xcc\xe5\x9b\x53\x4c\xd7\xeb\x63\xac\x62\xff\x6f\x0f\xe5\x2a\xda\xea\xdf\xf0\xcd\x4e\x94\x8e\xe4\xc6\xc9\xb5\x10\x29\x77\x54\x6a\x1a\x4b\x37\x95\x34\x17\xbd\x14\x9f\x27\xa2\x96\x85\x78\xc8\x8b\xdb\x1c\xf1\x4b\x17\xb2\x01\xe7\x34\x5f\x6d\x40\xe3\x90\xcf\x5c\xc5\x0a\x91\x23\x6c\xfa\x8e\x3b\x6b\x1a\x88\x42\x74\x83\x6f\xf7\x74\x41\x96\x9f\x00\x00\x00\xff\xff\x9f\xa8\x4d\x6c\xf6\x00\x00\x00")

func assetsDnsMetricsClusterRoleYamlBytes() ([]byte, error) {
	return bindataRead(
		_assetsDnsMetricsClusterRoleYaml,
		"assets/dns/metrics/cluster-role.yaml",
	)
}

func assetsDnsMetricsClusterRoleYaml() (*asset, error) {
	bytes, err := assetsDnsMetricsClusterRoleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "assets/dns/metrics/cluster-role.yaml", size: 246, mode: os.FileMode(420), modTime: time.Unix(1, 0)}
	a := &asset{bytes: bytes, info: info, digest: [32]uint8{0x64, 0xdb, 0xe0, 0x95, 0x65, 0xae, 0x53, 0x96, 0x3a, 0x5f, 0x5e, 0x8b, 0x69, 0xe2, 0x7d, 0x5, 0xbf, 0x1f, 0x3a, 0xf, 0xff, 0xd0, 0x6b, 0x23, 0x4f, 0xfd, 0x11, 0x7f, 0x57, 0xd4, 0x4a, 0x8b}}
	return a, nil
}

var _assetsDnsMetricsRoleBindingYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x94\xce\xb1\x4e\xc4\x40\x0c\x04\xd0\x7e\xbf\xc2\x3f\x90\x20\xba\xd3\x76\xd0\xd0\x1f\x12\xbd\x6f\xd7\x97\x98\x64\xed\x95\xed\x4d\xc1\xd7\x23\xa4\x48\x54\x20\x5d\x3b\x9a\xd1\x1b\xec\xfc\x41\xe6\xac\x92\xc1\x6e\x58\x66\x1c\xb1\xaa\xf1\x17\x06\xab\xcc\xdb\xc5\x67\xd6\xa7\xe3\x39\x6d\x2c\x35\xc3\x55\x77\x7a\x65\xa9\x2c\x4b\x6a\x14\x58\x31\x30\x27\x00\xc1\x46\x19\xba\x69\xa3\x58\x69\xf8\xb4\x5d\xfc\x8c\xbd\x63\xa1\x0c\xda\x49\x7c\xe5\x7b\x4c\x55\x3c\x99\xee\x74\xa5\xfb\xcf\x14\x3b\xbf\x99\x8e\xfe\x8f\x9f\x00\x7e\xf9\xbf\x34\x1f\xb7\x4f\x2a\xe1\x39\x4d\x67\xfb\x9d\xec\xe0\x42\x2f\xa5\xe8\x90\x78\xf0\x65\x53\xe1\x50\x63\x59\x20\x7d\x07\x00\x00\xff\xff\xb9\xd9\xab\x8d\x25\x01\x00\x00")

func assetsDnsMetricsRoleBindingYamlBytes() ([]byte, error) {
	return bindataRead(
		_assetsDnsMetricsRoleBindingYaml,
		"assets/dns/metrics/role-binding.yaml",
	)
}

func assetsDnsMetricsRoleBindingYaml() (*asset, error) {
	bytes, err := assetsDnsMetricsRoleBindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "assets/dns/metrics/role-binding.yaml", size: 293, mode: os.FileMode(420), modTime: time.Unix(1, 0)}
	a := &asset{bytes: bytes, info: info, digest: [32]uint8{0xc, 0x7d, 0xc7, 0x45, 0x33, 0xc4, 0xd8, 0xf, 0x8d, 0x89, 0x8d, 0x6, 0x47, 0xa7, 0xa, 0x6b, 0x17, 0xf5, 0x5f, 0x5a, 0x2f, 0xd8, 0xf9, 0x6, 0x71, 0xaa, 0x78, 0x8d, 0xb5, 0x7a, 0xf6, 0x99}}
	return a, nil
}

var _assetsDnsMetricsRoleYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x4c\x8e\xb1\x4e\xec\x40\x0c\x45\xfb\xf9\x0a\x6b\x5f\x9d\x7d\xa2\x5b\x4d\x8d\x44\x47\x01\x12\xbd\x77\xe6\x42\xac\x24\xe3\x91\xed\x04\xc1\xd7\xa3\xcd\x06\x89\xca\xf7\x1e\x59\x3e\xfe\x47\x2f\x3a\xc3\xa9\x01\x15\x95\xae\x5f\xd4\x4d\x17\xc4\x88\xd5\x29\x94\xbc\x18\x77\xd0\xe3\xf3\x2b\x2d\x08\x93\xe2\x84\x56\xbb\x4a\x8b\xc4\x5d\xde\x60\x2e\xda\x32\xd9\x95\xcb\x99\xd7\x18\xd5\xe4\x9b\x43\xb4\x9d\xa7\x8b\x9f\x45\xff\x6f\x0f\x69\x92\x56\xf3\x2e\x4a\x0b\x82\x2b\x07\xe7\x44\xd4\x78\x41\xfe\xe3\x1b\xa6\x8b\x1f\xd8\x3b\x17\x64\xd2\x8e\xe6\xa3\xbc\xc7\x50\x9b\x27\x5b\x67\x78\x4e\x03\x71\x97\x27\xd3\xb5\xfb\xed\xca\x40\xa7\x53\x22\x32\xb8\xae\x56\x70\x30\x87\x6d\x52\xe0\x7b\xf9\xfd\xf8\xde\xba\xd6\x5b\xd8\x60\xd7\x63\xf9\x03\xb1\xcf\x59\xfc\x1e\x3e\x39\xca\x98\x7e\x02\x00\x00\xff\xff\x29\x39\xda\x05\x1c\x01\x00\x00")

func assetsDnsMetricsRoleYamlBytes() ([]byte, error) {
	return bindataRead(
		_assetsDnsMetricsRoleYaml,
		"assets/dns/metrics/role.yaml",
	)
}

func assetsDnsMetricsRoleYaml() (*asset, error) {
	bytes, err := assetsDnsMetricsRoleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "assets/dns/metrics/role.yaml", size: 284, mode: os.FileMode(420), modTime: time.Unix(1, 0)}
	a := &asset{bytes: bytes, info: info, digest: [32]uint8{0x8c, 0xf2, 0x4e, 0x40, 0x91, 0xd8, 0x5e, 0x1c, 0x98, 0xb6, 0x2f, 0x11, 0x2a, 0x15, 0x8f, 0xe4, 0x7c, 0xfe, 0xc6, 0x31, 0xf3, 0xb2, 0xa0, 0x38, 0xb2, 0x3f, 0x15, 0x5a, 0x33, 0x12, 0xd2, 0x88}}
	return a, nil
}

var _assetsDnsNamespaceYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x64\x90\xcd\x4e\xc4\x30\x0c\x84\xef\x79\x8a\x51\x38\x2f\x3f\xd7\xbc\x03\x5c\x90\xb8\xbb\x8d\x97\x35\x4d\xed\x2a\x76\xcb\xeb\xa3\xb2\x15\xac\xb4\xc7\x68\x46\xf3\x7d\xf1\x24\x5a\x0b\xde\x68\x66\x5f\x68\xe4\x44\x8b\x7c\x70\x77\x31\x2d\xd8\x5e\xd2\xcc\x41\x95\x82\x4a\x02\x48\xd5\x82\x42\x4c\x7d\x7f\x02\xb6\xb0\xfa\x45\xce\xf1\x28\xf6\xa4\x56\xf9\xe4\xdc\x78\x0c\xeb\x05\x39\x27\x40\x69\xe6\xf2\x5f\x3b\x55\xf5\x04\x34\x1a\xb8\x1d\x13\x0f\x70\x0e\x6c\xd4\x56\x46\x18\x68\x33\xa9\xa8\xbc\xb0\x56\xd1\x4f\x98\x62\x5a\x07\x06\xd5\x59\x7c\x97\x42\x5c\x28\x8e\x82\xef\xf1\xdf\x38\x68\x11\xbf\xd7\xea\xab\x9e\x1a\x6f\xdc\x0a\xf2\x73\x3e\x98\xd4\x9a\x7d\xdf\x78\xcd\xa6\x12\xd6\x77\x62\x18\x9a\xd9\x84\xb3\x75\xbc\x73\xdf\x64\xe4\xd7\x6b\x0a\x1b\xbe\x78\x0c\x87\xec\x16\xe2\xbf\xbf\xbb\x1e\xed\x8e\x3a\xb6\xd5\x83\xfb\xcd\x70\x41\x8e\xbe\x72\x4e\x3f\x01\x00\x00\xff\xff\x82\x6d\x29\x03\x71\x01\x00\x00")

func assetsDnsNamespaceYamlBytes() ([]byte, error) {
	return bindataRead(
		_assetsDnsNamespaceYaml,
		"assets/dns/namespace.yaml",
	)
}

func assetsDnsNamespaceYaml() (*asset, error) {
	bytes, err := assetsDnsNamespaceYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "assets/dns/namespace.yaml", size: 369, mode: os.FileMode(420), modTime: time.Unix(1, 0)}
	a := &asset{bytes: bytes, info: info, digest: [32]uint8{0xe, 0xab, 0x50, 0x84, 0x61, 0x5f, 0x41, 0xf4, 0x17, 0x3b, 0x6, 0x84, 0xc0, 0x5f, 0x4f, 0xbb, 0xd8, 0x1d, 0xae, 0x26, 0x3e, 0x1f, 0x29, 0x2c, 0x84, 0x6d, 0x5e, 0xc1, 0x87, 0x97, 0x5f, 0xc9}}
	return a, nil
}

var _assetsDnsServiceAccountYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x2c\xc9\xb1\x09\xc4\x30\x0c\x05\xd0\xde\x53\x68\x81\x2b\xae\x55\x77\x33\x1c\xa4\x17\xf2\x0f\x11\xc1\xb2\xb1\x14\xcf\x1f\x02\xe9\x1e\xbc\xd3\xbc\x32\xfd\x31\x97\x29\x7e\xaa\xfd\xf2\x2c\x32\x6c\xc3\x0c\xeb\xce\xb4\xbe\xa5\x21\xa5\x4a\x0a\x17\x22\x97\x06\xa6\xea\xf1\x3a\x86\x28\x98\xfa\x80\xc7\x61\x7b\x7e\x9e\xba\x03\x00\x00\xff\xff\x8e\x2c\xf1\x2e\x55\x00\x00\x00")

func assetsDnsServiceAccountYamlBytes() ([]byte, error) {
	return bindataRead(
		_assetsDnsServiceAccountYaml,
		"assets/dns/service-account.yaml",
	)
}

func assetsDnsServiceAccountYaml() (*asset, error) {
	bytes, err := assetsDnsServiceAccountYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "assets/dns/service-account.yaml", size: 85, mode: os.FileMode(420), modTime: time.Unix(1, 0)}
	a := &asset{bytes: bytes, info: info, digest: [32]uint8{0x57, 0x12, 0x50, 0x4d, 0x67, 0x2f, 0x1b, 0x74, 0xa0, 0xa4, 0xbb, 0xa7, 0x59, 0xe9, 0x5a, 0xc6, 0xc1, 0x1a, 0xf8, 0x5f, 0xff, 0x5, 0xdb, 0xc, 0x10, 0x8b, 0xc1, 0x0, 0xcc, 0xf, 0x9f, 0x3a}}
	return a, nil
}

var _assetsDnsServiceYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x84\x91\x31\x6f\xe2\x40\x10\x85\x7b\xff\x8a\x27\xe8\x4e\xc0\x09\xdd\x51\x9c\xdb\xa3\x89\x52\x80\x14\x48\x3f\x5e\x4f\xcc\x8a\xf5\x8c\xb5\x33\x06\xf1\xef\x23\x4c\x42\x80\x14\x69\x56\xda\x7d\x9f\x3e\x3d\xbd\xdd\x47\xa9\x4b\xbc\x70\x3e\xc4\xc0\x05\x75\xf1\x95\xb3\x45\x95\x12\x87\x79\x31\x86\x50\xcb\x93\xe1\xb4\x8e\x02\x4f\x12\x55\x9c\x0c\x24\x35\x48\x44\x9d\x3c\xaa\x18\x28\x33\x8c\x1d\xe4\xc8\xbd\x78\x6c\xb9\xb0\x8e\x43\x59\x00\x63\x84\xd4\x9b\x73\x7e\x5a\xe3\x18\x53\x42\xc5\xa0\xde\xb5\x25\x8f\x81\x52\x3a\xa1\x25\xa1\x86\xeb\xd9\x00\x1b\x27\x0e\xae\x19\xd1\x1e\x8d\x40\xa7\xd9\xed\x2c\x9d\x0e\x95\x4a\xd4\x62\x05\x70\x09\x4a\x2c\xfe\x0c\x17\xa7\xdc\xb0\xaf\x87\xa7\x2b\x90\xd5\x35\x68\x2a\xb1\x5d\xae\xef\x05\x53\x0f\xdd\x8f\x92\x2f\xe8\x2a\xda\xfc\xbf\x15\xb5\xec\x39\x86\xdb\x36\xff\xe6\x8b\xbf\xdf\x54\x77\xd8\x83\x6a\x8c\xcd\x6a\xb9\x2a\xb1\x95\xa0\x6d\xcb\xe2\x38\xee\x58\x60\x97\xbf\x81\x6b\xa7\x49\x9b\x13\xde\x98\xbc\xcf\x8c\x86\x9c\xcf\x33\xb1\x50\x95\x3e\xf6\xfb\x84\x9e\xf9\x64\x97\xf5\x31\xc5\x68\xdf\x57\x9c\x85\x9d\x6d\x16\xf5\xf7\x4e\xcd\xcf\xa5\x47\xd7\xfc\xd7\xa8\x78\x0f\x00\x00\xff\xff\x82\x42\x75\xa4\x08\x02\x00\x00")

func assetsDnsServiceYamlBytes() ([]byte, error) {
	return bindataRead(
		_assetsDnsServiceYaml,
		"assets/dns/service.yaml",
	)
}

func assetsDnsServiceYaml() (*asset, error) {
	bytes, err := assetsDnsServiceYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "assets/dns/service.yaml", size: 520, mode: os.FileMode(420), modTime: time.Unix(1, 0)}
	a := &asset{bytes: bytes, info: info, digest: [32]uint8{0x18, 0x69, 0xc5, 0xf1, 0xe, 0xc, 0x77, 0xe5, 0x78, 0xce, 0xfc, 0xc2, 0x41, 0xf8, 0x21, 0x87, 0x8a, 0xb7, 0x67, 0xdd, 0x48, 0x94, 0x63, 0x79, 0x69, 0x4e, 0x38, 0x53, 0x3c, 0xdb, 0xc7, 0x13}}
	return a, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	canonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[canonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("Asset %s can't read by error: %v", name, err)
		}
		return a.bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// AssetString returns the asset contents as a string (instead of a []byte).
func AssetString(name string) (string, error) {
	data, err := Asset(name)
	return string(data), err
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	a, err := Asset(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return a
}

// MustAssetString is like AssetString but panics when Asset would return an
// error. It simplifies safe initialization of global variables.
func MustAssetString(name string) string {
	return string(MustAsset(name))
}

// AssetInfo loads and returns the asset info for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func AssetInfo(name string) (os.FileInfo, error) {
	canonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[canonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("AssetInfo %s can't read by error: %v", name, err)
		}
		return a.info, nil
	}
	return nil, fmt.Errorf("AssetInfo %s not found", name)
}

// AssetDigest returns the digest of the file with the given name. It returns an
// error if the asset could not be found or the digest could not be loaded.
func AssetDigest(name string) ([sha256.Size]byte, error) {
	canonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[canonicalName]; ok {
		a, err := f()
		if err != nil {
			return [sha256.Size]byte{}, fmt.Errorf("AssetDigest %s can't read by error: %v", name, err)
		}
		return a.digest, nil
	}
	return [sha256.Size]byte{}, fmt.Errorf("AssetDigest %s not found", name)
}

// Digests returns a map of all known files and their checksums.
func Digests() (map[string][sha256.Size]byte, error) {
	mp := make(map[string][sha256.Size]byte, len(_bindata))
	for name := range _bindata {
		a, err := _bindata[name]()
		if err != nil {
			return nil, err
		}
		mp[name] = a.digest
	}
	return mp, nil
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() (*asset, error){
	"assets/dns/cluster-role-binding.yaml": assetsDnsClusterRoleBindingYaml,

	"assets/dns/cluster-role.yaml": assetsDnsClusterRoleYaml,

	"assets/dns/daemonset.yaml": assetsDnsDaemonsetYaml,

	"assets/dns/metrics/cluster-role-binding.yaml": assetsDnsMetricsClusterRoleBindingYaml,

	"assets/dns/metrics/cluster-role.yaml": assetsDnsMetricsClusterRoleYaml,

	"assets/dns/metrics/role-binding.yaml": assetsDnsMetricsRoleBindingYaml,

	"assets/dns/metrics/role.yaml": assetsDnsMetricsRoleYaml,

	"assets/dns/namespace.yaml": assetsDnsNamespaceYaml,

	"assets/dns/service-account.yaml": assetsDnsServiceAccountYaml,

	"assets/dns/service.yaml": assetsDnsServiceYaml,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"},
// AssetDir("data/img") would return []string{"a.png", "b.png"},
// AssetDir("foo.txt") and AssetDir("notexist") would return an error, and
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		canonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(canonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for childName := range node.Children {
		rv = append(rv, childName)
	}
	return rv, nil
}

type bintree struct {
	Func     func() (*asset, error)
	Children map[string]*bintree
}

var _bintree = &bintree{nil, map[string]*bintree{
	"assets": {nil, map[string]*bintree{
		"dns": {nil, map[string]*bintree{
			"cluster-role-binding.yaml": {assetsDnsClusterRoleBindingYaml, map[string]*bintree{}},
			"cluster-role.yaml":         {assetsDnsClusterRoleYaml, map[string]*bintree{}},
			"daemonset.yaml":            {assetsDnsDaemonsetYaml, map[string]*bintree{}},
			"metrics": {nil, map[string]*bintree{
				"cluster-role-binding.yaml": {assetsDnsMetricsClusterRoleBindingYaml, map[string]*bintree{}},
				"cluster-role.yaml":         {assetsDnsMetricsClusterRoleYaml, map[string]*bintree{}},
				"role-binding.yaml":         {assetsDnsMetricsRoleBindingYaml, map[string]*bintree{}},
				"role.yaml":                 {assetsDnsMetricsRoleYaml, map[string]*bintree{}},
			}},
			"namespace.yaml":       {assetsDnsNamespaceYaml, map[string]*bintree{}},
			"service-account.yaml": {assetsDnsServiceAccountYaml, map[string]*bintree{}},
			"service.yaml":         {assetsDnsServiceYaml, map[string]*bintree{}},
		}},
	}},
}}

// RestoreAsset restores an asset under the given directory.
func RestoreAsset(dir, name string) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	info, err := AssetInfo(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(_filePath(dir, filepath.Dir(name)), os.FileMode(0755))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(_filePath(dir, name), data, info.Mode())
	if err != nil {
		return err
	}
	return os.Chtimes(_filePath(dir, name), info.ModTime(), info.ModTime())
}

// RestoreAssets restores an asset under the given directory recursively.
func RestoreAssets(dir, name string) error {
	children, err := AssetDir(name)
	// File
	if err != nil {
		return RestoreAsset(dir, name)
	}
	// Dir
	for _, child := range children {
		err = RestoreAssets(dir, filepath.Join(name, child))
		if err != nil {
			return err
		}
	}
	return nil
}

func _filePath(dir, name string) string {
	canonicalName := strings.Replace(name, "\\", "/", -1)
	return filepath.Join(append([]string{dir}, strings.Split(canonicalName, "/")...)...)
}
