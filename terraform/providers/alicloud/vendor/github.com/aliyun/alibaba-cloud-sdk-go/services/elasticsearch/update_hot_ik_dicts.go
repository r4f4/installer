package elasticsearch

//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.
//
// Code generated by Alibaba Cloud SDK Code Generator.
// Changes may cause incorrect behavior and will be lost if the code is regenerated.

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
)

// UpdateHotIkDicts invokes the elasticsearch.UpdateHotIkDicts API synchronously
func (client *Client) UpdateHotIkDicts(request *UpdateHotIkDictsRequest) (response *UpdateHotIkDictsResponse, err error) {
	response = CreateUpdateHotIkDictsResponse()
	err = client.DoAction(request, response)
	return
}

// UpdateHotIkDictsWithChan invokes the elasticsearch.UpdateHotIkDicts API asynchronously
func (client *Client) UpdateHotIkDictsWithChan(request *UpdateHotIkDictsRequest) (<-chan *UpdateHotIkDictsResponse, <-chan error) {
	responseChan := make(chan *UpdateHotIkDictsResponse, 1)
	errChan := make(chan error, 1)
	err := client.AddAsyncTask(func() {
		defer close(responseChan)
		defer close(errChan)
		response, err := client.UpdateHotIkDicts(request)
		if err != nil {
			errChan <- err
		} else {
			responseChan <- response
		}
	})
	if err != nil {
		errChan <- err
		close(responseChan)
		close(errChan)
	}
	return responseChan, errChan
}

// UpdateHotIkDictsWithCallback invokes the elasticsearch.UpdateHotIkDicts API asynchronously
func (client *Client) UpdateHotIkDictsWithCallback(request *UpdateHotIkDictsRequest, callback func(response *UpdateHotIkDictsResponse, err error)) <-chan int {
	result := make(chan int, 1)
	err := client.AddAsyncTask(func() {
		var response *UpdateHotIkDictsResponse
		var err error
		defer close(result)
		response, err = client.UpdateHotIkDicts(request)
		callback(response, err)
		result <- 1
	})
	if err != nil {
		defer close(result)
		callback(nil, err)
		result <- 0
	}
	return result
}

// UpdateHotIkDictsRequest is the request struct for api UpdateHotIkDicts
type UpdateHotIkDictsRequest struct {
	*requests.RoaRequest
	InstanceId  string `position:"Path" name:"InstanceId"`
	ClientToken string `position:"Query" name:"clientToken"`
}

// UpdateHotIkDictsResponse is the response struct for api UpdateHotIkDicts
type UpdateHotIkDictsResponse struct {
	*responses.BaseResponse
	RequestId string     `json:"RequestId" xml:"RequestId"`
	Result    []DictList `json:"Result" xml:"Result"`
}

// CreateUpdateHotIkDictsRequest creates a request to invoke UpdateHotIkDicts API
func CreateUpdateHotIkDictsRequest() (request *UpdateHotIkDictsRequest) {
	request = &UpdateHotIkDictsRequest{
		RoaRequest: &requests.RoaRequest{},
	}
	request.InitWithApiInfo("elasticsearch", "2017-06-13", "UpdateHotIkDicts", "/openapi/instances/[InstanceId]/ik-hot-dict", "elasticsearch", "openAPI")
	request.Method = requests.PUT
	return
}

// CreateUpdateHotIkDictsResponse creates a response to parse from UpdateHotIkDicts response
func CreateUpdateHotIkDictsResponse() (response *UpdateHotIkDictsResponse) {
	response = &UpdateHotIkDictsResponse{
		BaseResponse: &responses.BaseResponse{},
	}
	return
}