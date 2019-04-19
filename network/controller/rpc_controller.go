//
// Modified BSD 3-Clause Clear License
//
// Copyright (c) 2019 Insolar Technologies GmbH
//
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without modification,
// are permitted (subject to the limitations in the disclaimer below) provided that
// the following conditions are met:
//  * Redistributions of source code must retain the above copyright notice, this list
//    of conditions and the following disclaimer.
//  * Redistributions in binary form must reproduce the above copyright notice, this list
//    of conditions and the following disclaimer in the documentation and/or other materials
//    provided with the distribution.
//  * Neither the name of Insolar Technologies GmbH nor the names of its contributors
//    may be used to endorse or promote products derived from this software without
//    specific prior written permission.
//
// NO EXPRESS OR IMPLIED LICENSES TO ANY PARTY'S PATENT RIGHTS ARE GRANTED
// BY THIS LICENSE. THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS
// AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES,
// INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY
// AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL
// THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT,
// INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING,
// BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS
// OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
// ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
//
// Notwithstanding any other provisions of this license, it is prohibited to:
//    (a) use this software,
//
//    (b) prepare modifications and derivative works of this software,
//
//    (c) distribute this software (including without limitation in source code, binary or
//        object code form), and
//
//    (d) reproduce copies of this software
//
//    for any commercial purposes, and/or
//
//    for the purposes of making available this software to third parties as a service,
//    including, without limitation, any software-as-a-service, platform-as-a-service,
//    infrastructure-as-a-service or other similar online service, irrespective of
//    whether it competes with the products or services of Insolar Technologies GmbH.
//

package controller

import (
	"context"
	"encoding/gob"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"

	"github.com/insolar/insolar/component"
	"github.com/insolar/insolar/insolar"
	"github.com/insolar/insolar/insolar/message"
	"github.com/insolar/insolar/instrumentation/inslogger"
	"github.com/insolar/insolar/instrumentation/insmetrics"
	"github.com/insolar/insolar/instrumentation/instracer"
	"github.com/insolar/insolar/network"
	"github.com/insolar/insolar/network/cascade"
	"github.com/insolar/insolar/network/controller/common"
	"github.com/insolar/insolar/network/hostnetwork/packet/types"
)

type RPCController interface {
	component.Initer

	// hack for DI, else we receive ServiceNetwork injection in RPCController instead of rpcController that leads to stack overflow
	IAmRPCController()

	SendMessage(nodeID insolar.Reference, name string, msg insolar.Parcel) ([]byte, error)
	SendBytes(ctx context.Context, nodeID insolar.Reference, name string, msgBytes []byte) ([]byte, error)
	SendCascadeMessage(data insolar.Cascade, method string, msg insolar.Parcel) error
	RemoteProcedureRegister(name string, method insolar.RemoteProcedure)
}

type rpcController struct {
	Scheme  insolar.PlatformCryptographyScheme `inject:""`
	Network network.HostNetwork                `inject:""`

	options     *common.Options
	methodTable map[string]insolar.RemoteProcedure
}

type RequestRPC struct {
	Method string
	Data   [][]byte
}

type ResponseRPC struct {
	Success bool
	Result  []byte
	Error   string
}

type RequestCascade struct {
	TraceID string
	RPC     RequestRPC
	Cascade insolar.Cascade
}

type ResponseCascade struct {
	Success bool
	Error   string
}

func init() {
	gob.Register(&RequestRPC{})
	gob.Register(&ResponseRPC{})
	gob.Register(&RequestCascade{})
	gob.Register(&ResponseCascade{})
}

func (rpc *rpcController) IAmRPCController() {
	// hack for DI, else we receive ServiceNetwork injection in RPCController instead of rpcController that leads to stack overflow
}

func (rpc *rpcController) RemoteProcedureRegister(name string, method insolar.RemoteProcedure) {
	_, span := instracer.StartSpan(context.Background(), "RPCController.RemoteProcedureRegister")
	span.AddAttributes(
		trace.StringAttribute("method", name),
	)
	defer span.End()
	fmt.Println("add love: ", name)
	rpc.methodTable[name] = method
}

func (rpc *rpcController) invoke(ctx context.Context, name string, data [][]byte) ([]byte, error) {
	fmt.Println("is love?: ", name)
	method, exists := rpc.methodTable[name]
	if !exists {
		fmt.Println("is love?: no(")
		return nil, errors.New(fmt.Sprintf("RPC with name %s is not registered", name))
	}
	fmt.Println("is love?: yes)")
	return method(ctx, data)
}

func (rpc *rpcController) SendCascadeMessage(data insolar.Cascade, method string, msg insolar.Parcel) error {
	if msg == nil {
		return errors.New("message is nil")
	}
	ctx, span := instracer.StartSpan(context.Background(), "RPCController.SendCascadeMessage")
	span.AddAttributes(
		trace.StringAttribute("method", method),
		trace.StringAttribute("msg.Type", msg.Type().String()),
		trace.StringAttribute("msg.DefaultTarget", msg.DefaultTarget().String()),
	)
	defer span.End()
	ctx = msg.Context(ctx)
	return rpc.initCascadeSendMessage(ctx, data, false, method, [][]byte{message.ParcelToBytes(msg)})
}

func (rpc *rpcController) initCascadeSendMessage(ctx context.Context, data insolar.Cascade,
	findCurrentNode bool, method string, args [][]byte) error {

	_, span := instracer.StartSpan(context.Background(), "RPCController.initCascadeSendMessage")
	span.AddAttributes(
		trace.StringAttribute("method", method),
	)
	defer span.End()
	if len(data.NodeIds) == 0 {
		return errors.New("node IDs list should not be empty")
	}
	if data.ReplicationFactor == 0 {
		return errors.New("replication factor should not be zero")
	}

	var nextNodes []insolar.Reference
	var err error

	if findCurrentNode {
		nodeID := rpc.Network.GetNodeID()
		nextNodes, err = cascade.CalculateNextNodes(rpc.Scheme, data, &nodeID)
	} else {
		nextNodes, err = cascade.CalculateNextNodes(rpc.Scheme, data, nil)
	}
	if err != nil {
		return errors.Wrap(err, "Failed to CalculateNextNodes")
	}
	if len(nextNodes) == 0 {
		return nil
	}

	var failedNodes []string
	for _, nextNode := range nextNodes {
		err = rpc.requestCascadeSendMessage(ctx, data, nextNode, method, args)
		if err != nil {
			inslogger.FromContext(ctx).Warnf("Failed to send cascade message to node %s: %s", nextNode, err.Error())
			failedNodes = append(failedNodes, nextNode.String())
		}
	}

	if len(failedNodes) > 0 {
		return errors.New("Failed to send cascade message to nodes: " + strings.Join(failedNodes, ", "))
	}
	inslogger.FromContext(ctx).Debug("Cascade message successfully sent to all nodes of the next layer")
	return nil
}

func (rpc *rpcController) requestCascadeSendMessage(ctx context.Context, data insolar.Cascade, nodeID insolar.Reference,
	method string, args [][]byte) error {

	_, span := instracer.StartSpan(context.Background(), "RPCController.requestCascadeSendMessage")
	defer span.End()
	request := rpc.Network.NewRequestBuilder().Type(types.Cascade).Data(&RequestCascade{
		TraceID: inslogger.TraceID(ctx),
		RPC: RequestRPC{
			Method: method,
			Data:   args,
		},
		Cascade: data,
	}).Build()

	future, err := rpc.Network.SendRequest(ctx, request, nodeID)
	if err != nil {
		return err
	}

	go func(ctx context.Context, f network.Future, duration time.Duration) {
		response, err := f.GetResponse(duration)
		if err != nil {
			inslogger.FromContext(ctx).Warnf("Failed to get response to cascade message request from node %s: %s",
				future.GetRequest().GetSender(), err.Error())
			return
		}
		data := response.GetData().(*ResponseCascade)
		if !data.Success {
			inslogger.FromContext(ctx).Warnf("Error response to cascade message request from node %s: %s",
				response.GetSender(), data.Error)
			return
		}
	}(ctx, future, rpc.options.PacketTimeout)

	return nil
}

func (rpc *rpcController) SendBytes(ctx context.Context, nodeID insolar.Reference, name string, msgBytes []byte) ([]byte, error) {
	request := rpc.Network.NewRequestBuilder().Type(types.RPC).Data(&RequestRPC{
		Method: name,
		Data:   [][]byte{msgBytes},
	}).Build()

	logger := inslogger.FromContext(ctx)
	logger.Debugf("SendParcel with nodeID = %s method = %s, RequestID = %d", nodeID.String(),
		name, request.GetRequestID())
	future, err := rpc.Network.SendRequest(ctx, request, nodeID)
	if err != nil {
		return nil, errors.Wrapf(err, "Error sending RPC request to node %s", nodeID.String())
	}
	response, err := future.GetResponse(rpc.options.PacketTimeout)
	if err != nil {
		return nil, errors.Wrapf(err, "Error getting RPC response from node %s", nodeID.String())
	}
	data := response.GetData().(*ResponseRPC)
	if !data.Success {
		return nil, errors.New("RPC call returned error: " + data.Error)
	}
	stats.Record(ctx, statParcelsReplySizeBytes.M(int64(len(data.Result))))
	return data.Result, nil
}

func (rpc *rpcController) SendMessage(nodeID insolar.Reference, name string, msg insolar.Parcel) ([]byte, error) {
	msgBytes := message.ParcelToBytes(msg)
	ctx := context.Background() // TODO: ctx as argument
	ctx = insmetrics.InsertTag(ctx, tagMessageType, msg.Type().String())
	stats.Record(ctx, statParcelsSentSizeBytes.M(int64(len(msgBytes))))
	request := rpc.Network.NewRequestBuilder().Type(types.RPC).Data(&RequestRPC{
		Method: name,
		Data:   [][]byte{msgBytes},
	}).Build()

	start := time.Now()
	ctx = msg.Context(ctx)
	logger := inslogger.FromContext(ctx)
	logger.Debugf("SendParcel with nodeID = %s method = %s, message reference = %s, RequestID = %d", nodeID.String(),
		name, msg.DefaultTarget().String(), request.GetRequestID())
	future, err := rpc.Network.SendRequest(ctx, request, nodeID)
	if err != nil {
		return nil, errors.Wrapf(err, "Error sending RPC request to node %s", nodeID.String())
	}
	response, err := future.GetResponse(rpc.options.PacketTimeout)
	if err != nil {
		return nil, errors.Wrapf(err, "Error getting RPC response from node %s", nodeID.String())
	}
	data := response.GetData().(*ResponseRPC)
	logger.Debugf("Inside SendParcel: type - '%s', target - %s, caller - %s, targetRole - %s, time - %s",
		msg.Type(), msg.DefaultTarget(), msg.GetCaller(), msg.DefaultRole(), time.Since(start))
	if !data.Success {
		return nil, errors.New("RPC call returned error: " + data.Error)
	}
	stats.Record(ctx, statParcelsReplySizeBytes.M(int64(len(data.Result))))
	return data.Result, nil
}

func (rpc *rpcController) processMessage(ctx context.Context, request network.Request) (network.Response, error) {
	ctx = insmetrics.InsertTag(ctx, tagPacketType, request.GetType().String())
	stats.Record(ctx, statPacketsReceived.M(1))

	payload := request.GetData().(*RequestRPC)
	result, err := rpc.invoke(ctx, payload.Method, payload.Data)
	if err != nil {
		return rpc.Network.BuildResponse(ctx, request, &ResponseRPC{Success: false, Error: err.Error()}), nil
	}
	return rpc.Network.BuildResponse(ctx, request, &ResponseRPC{Success: true, Result: result}), nil
}

func (rpc *rpcController) processCascade(ctx context.Context, request network.Request) (network.Response, error) {
	payload := request.GetData().(*RequestCascade)
	ctx, logger := inslogger.WithTraceField(ctx, payload.TraceID)

	generalError := ""
	_, invokeErr := rpc.invoke(ctx, payload.RPC.Method, payload.RPC.Data)
	if invokeErr != nil {
		logger.Debugf("failed to invoke RPC: %s", invokeErr.Error())
		generalError += invokeErr.Error() + "; "
	}
	sendErr := rpc.initCascadeSendMessage(ctx, payload.Cascade, true, payload.RPC.Method, payload.RPC.Data)
	if sendErr != nil {
		logger.Debugf("failed to send message to next cascade layer: %s", sendErr.Error())
		generalError += sendErr.Error()
	}

	if generalError != "" {
		return rpc.Network.BuildResponse(ctx, request, &ResponseCascade{Success: false, Error: generalError}), nil
	}
	return rpc.Network.BuildResponse(ctx, request, &ResponseCascade{Success: true}), nil
}

func (rpc *rpcController) Init(ctx context.Context) error {
	rpc.Network.RegisterRequestHandler(types.RPC, rpc.processMessage)
	rpc.Network.RegisterRequestHandler(types.Cascade, rpc.processCascade)
	return nil
}

func NewRPCController(options *common.Options) RPCController {
	return &rpcController{options: options, methodTable: make(map[string]insolar.RemoteProcedure)}
}
