package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/alibabacloud-go/tea/tea"
)

var filterKey = []string{"x-ca-signature", "x-ca-signature-headers", "accept", "content-md5", "content-type", "date", "host", "token"}

// Sorter defines the key-value structure for storing the sorted data in signHeader.
type Sorter struct {
	Keys []string
	Vals []string
}

// newSorter is an additional function for function Sign.
func newSorter(m map[string]*string) *Sorter {
	hs := &Sorter{
		Keys: make([]string, 0, len(m)),
		Vals: make([]string, 0, len(m)),
	}

	for k, v := range m {
		hs.Keys = append(hs.Keys, k)
		hs.Vals = append(hs.Vals, tea.StringValue(v))
	}
	return hs
}

// Sort is an additional function for function SignHeader.
func (hs *Sorter) Sort() {
	sort.Sort(hs)
}

// Len is an additional function for function SignHeader.
func (hs *Sorter) Len() int {
	return len(hs.Vals)
}

// Less is an additional function for function SignHeader.
func (hs *Sorter) Less(i, j int) bool {
	return bytes.Compare([]byte(hs.Keys[i]), []byte(hs.Keys[j])) < 0
}

// Swap is an additional function for function SignHeader.
func (hs *Sorter) Swap(i, j int) {
	hs.Vals[i], hs.Vals[j] = hs.Vals[j], hs.Vals[i]
	hs.Keys[i], hs.Keys[j] = hs.Keys[j], hs.Keys[i]
}

func flatRepeatedList(dataValue reflect.Value, result map[string]*string, prefix string) {
	if !dataValue.IsValid() {
		return
	}

	dataType := dataValue.Type()
	if dataType.Kind().String() == "slice" {
		handleRepeatedParams(dataValue, result, prefix)
	} else if dataType.Kind().String() == "map" {
		handleMap(dataValue, result, prefix)
	} else {
		result[prefix] = tea.String(fmt.Sprintf("%v", dataValue.Interface()))
	}
}

func handleRepeatedParams(repeatedFieldValue reflect.Value, result map[string]*string, prefix string) {
	if repeatedFieldValue.IsValid() && !repeatedFieldValue.IsNil() {
		for m := 0; m < repeatedFieldValue.Len(); m++ {
			elementValue := repeatedFieldValue.Index(m)
			key := prefix + "." + strconv.Itoa(m+1)
			fieldValue := reflect.ValueOf(elementValue.Interface())
			if fieldValue.Kind().String() == "map" {
				handleMap(fieldValue, result, key)
			} else {
				result[key] = tea.String(fmt.Sprintf("%v", fieldValue.Interface()))
			}
		}
	}
}

func handleMap(valueField reflect.Value, result map[string]*string, prefix string) {
	if valueField.IsValid() && valueField.String() != "" {
		valueFieldType := valueField.Type()
		if valueFieldType.Kind().String() == "map" {
			var byt []byte
			byt, _ = json.Marshal(valueField.Interface())
			cache := make(map[string]interface{})
			_ = json.Unmarshal(byt, &cache)
			for key, value := range cache {
				pre := ""
				if prefix != "" {
					pre = prefix + "." + key
				} else {
					pre = key
				}
				fieldValue := reflect.ValueOf(value)
				flatRepeatedList(fieldValue, result, pre)
			}
		}
	}
}

func getSignature(appSecret string, req *tea.Request) string {
	signedHeader := getSignedHeader(req)
	url := buildUrl(req)
	date := tea.StringValue(req.Headers["date"])
	accept := tea.StringValue(req.Headers["accept"])
	contentType := tea.StringValue(req.Headers["content-type"])
	contentMd5 := tea.StringValue(req.Headers["content-md5"])
	signStr := tea.StringValue(req.Method) + "\n" + accept + "\n" + contentMd5 + "\n" + contentType + "\n" + date + "\n" + signedHeader + "\n" + url
	h := hmac.New(func() hash.Hash { return sha256.New() }, []byte(appSecret))
	io.WriteString(h, signStr)
	fmt.Println("signStr: ", signStr)
	signedStr := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return signedStr
}

func getSignedHeader(request *tea.Request) string {
	signedHeader := ""
	signedHeaderKeys := ""
	hs := newSorter(request.Headers)
	hs.Sort()
	for key, value := range hs.Keys {
		if !isFilterKey(value) {
			signedHeaderKeys += value + ","
			signedHeader += value + ":" + hs.Vals[key] + "\n"
		}
	}
	request.Headers["x-ca-signature-headers"] = tea.String(strings.TrimSuffix(signedHeaderKeys, ","))
	return strings.TrimSuffix(signedHeader, "\n")
}

func isFilterKey(key string) bool {
	for _, value := range filterKey {
		if key == value {
			return true
		}
	}
	return false
}

func buildUrl(request *tea.Request) string {
	urlPath := tea.StringValue(request.Pathname)
	hs := newSorter(request.Query)
	if request.Method != nil && *request.Method == http.MethodPost {
		var (
			tmp = map[string]*string{}
			buf bytes.Buffer
		)
		io.Copy(&buf, request.Body)

		request.Body = strings.NewReader(buf.String())
		vals, _ := url.ParseQuery(buf.String())
		for k, _ := range vals {
			tmp[k] = tea.String(vals.Get(k))
		}
		hs = newSorter(tmp)
	}

	hs.Sort()
	if len(hs.Keys) > 0 {
		urlPath += "?"
	}
	for key, value := range hs.Keys {
		if !strings.HasSuffix(urlPath, "?") {
			urlPath += "&"
		}
		urlPath += value + "=" + hs.Vals[key]
	}

	return urlPath
}
