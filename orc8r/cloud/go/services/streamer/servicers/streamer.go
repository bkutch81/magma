/*
Copyright (c) Facebook, Inc. and its affiliates.
All rights reserved.

This source code is licensed under the BSD-style license found in the
LICENSE file in the root directory of this source tree.
*/

/*
StreamingServer acts as gRPC service for all applications that require
to be notified on database changes with a stream of updates.

The applications in the gateways connect to the StreamingServer and
the updates would be pushed through that channel.
*/
package servicers

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"magma/orc8r/cloud/go/identity"
	"magma/orc8r/cloud/go/services/streamer"
	"magma/orc8r/cloud/go/services/streamer/providers"
	"magma/orc8r/lib/go/protos"
)

type StreamingServer struct{}

// GetUpdatesUnverified finds a stream provider for stream name in the request and starts streaming updates
// generated by the provider
// If provider's GetUpdates() returns error == nil, the update batch is sent to the client and the stream is closed
// If provider's GetUpdates() returns error == EAGAIN, the update batch is sent to the client and the streaming
// continues on the same stream
func GetUpdatesUnverified(request *protos.StreamRequest, stream protos.Streamer_GetUpdatesServer) error {
	streamProvider, err := providers.GetStreamProvider(request.GetStreamName())
	if err != nil {
		return status.Errorf(codes.Unavailable, "Stream %s does not exist", request.GetStreamName())
	}
	var updates []*protos.DataUpdate
	for err = streamer.EAGAIN; err == streamer.EAGAIN; {
		updates, err = streamProvider.GetUpdates(request.GetGatewayId(), request.ExtraArgs)
		if err != nil && err != streamer.EAGAIN {
			return status.Errorf(codes.Aborted, "Error while streaming updates: %s", err)
		}
		updateBatch := new(protos.DataUpdateBatch)
		updateBatch.Resync = true
		updateBatch.Updates = updates
		stream.Send(updateBatch)
	}
	return nil
}

// GetUpdates populates GW HwId in the request from GRPC metadata, finds a stream provider for stream name in
// the request and starts streaming updates generated by the provider. See GetUpdatesUnverified() for more details
func (srv *StreamingServer) GetUpdates(request *protos.StreamRequest, stream protos.Streamer_GetUpdatesServer) error {
	if request == nil {
		return status.Error(codes.InvalidArgument, "nil request")
	}
	// Check if we can get a valid Gateway identity
	gwIdentity, err := identity.GetStreamGatewayId(stream)
	if err != nil {
		return err
	}
	if gwIdentity.HardwareId == "" {
		return status.Errorf(codes.FailedPrecondition, "Gateway ID is empty")
	}
	// Overwrite/set Gw Id using verified identity from Certifier.
	// Older Gateways will populate their own Hw Id while the newer
	// Gateways may avoid doing so. We should be working with verified
	// identities in both cases or reject the request if there is none.
	request.GatewayId = gwIdentity.HardwareId
	return GetUpdatesUnverified(request, stream)
}
