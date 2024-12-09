// package: wox.plugin
// file: plugin.proto

/* tslint:disable */
/* eslint-disable */

import * as grpc from "grpc";
import * as plugin_pb from "./plugin_pb";
import * as common_pb from "./common_pb";
import * as api_pb from "./api_pb";

interface IPluginService extends grpc.ServiceDefinition<grpc.UntypedServiceImplementation> {
    init: IPluginService_IInit;
    query: IPluginService_IQuery;
}

interface IPluginService_IInit extends grpc.MethodDefinition<plugin_pb.PluginInitContext, common_pb.Empty> {
    path: "/wox.plugin.Plugin/Init";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<plugin_pb.PluginInitContext>;
    requestDeserialize: grpc.deserialize<plugin_pb.PluginInitContext>;
    responseSerialize: grpc.serialize<common_pb.Empty>;
    responseDeserialize: grpc.deserialize<common_pb.Empty>;
}
interface IPluginService_IQuery extends grpc.MethodDefinition<plugin_pb.PluginQueryContext, plugin_pb.QueryResponse> {
    path: "/wox.plugin.Plugin/Query";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<plugin_pb.PluginQueryContext>;
    requestDeserialize: grpc.deserialize<plugin_pb.PluginQueryContext>;
    responseSerialize: grpc.serialize<plugin_pb.QueryResponse>;
    responseDeserialize: grpc.deserialize<plugin_pb.QueryResponse>;
}

export const PluginService: IPluginService;

export interface IPluginServer {
    init: grpc.handleUnaryCall<plugin_pb.PluginInitContext, common_pb.Empty>;
    query: grpc.handleUnaryCall<plugin_pb.PluginQueryContext, plugin_pb.QueryResponse>;
}

export interface IPluginClient {
    init(request: plugin_pb.PluginInitContext, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    init(request: plugin_pb.PluginInitContext, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    init(request: plugin_pb.PluginInitContext, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    query(request: plugin_pb.PluginQueryContext, callback: (error: grpc.ServiceError | null, response: plugin_pb.QueryResponse) => void): grpc.ClientUnaryCall;
    query(request: plugin_pb.PluginQueryContext, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: plugin_pb.QueryResponse) => void): grpc.ClientUnaryCall;
    query(request: plugin_pb.PluginQueryContext, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: plugin_pb.QueryResponse) => void): grpc.ClientUnaryCall;
}

export class PluginClient extends grpc.Client implements IPluginClient {
    constructor(address: string, credentials: grpc.ChannelCredentials, options?: object);
    public init(request: plugin_pb.PluginInitContext, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public init(request: plugin_pb.PluginInitContext, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public init(request: plugin_pb.PluginInitContext, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public query(request: plugin_pb.PluginQueryContext, callback: (error: grpc.ServiceError | null, response: plugin_pb.QueryResponse) => void): grpc.ClientUnaryCall;
    public query(request: plugin_pb.PluginQueryContext, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: plugin_pb.QueryResponse) => void): grpc.ClientUnaryCall;
    public query(request: plugin_pb.PluginQueryContext, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: plugin_pb.QueryResponse) => void): grpc.ClientUnaryCall;
}

interface ISystemPluginService extends grpc.ServiceDefinition<grpc.UntypedServiceImplementation> {
    getMetadata: ISystemPluginService_IGetMetadata;
}

interface ISystemPluginService_IGetMetadata extends grpc.MethodDefinition<common_pb.Empty, plugin_pb.Metadata> {
    path: "/wox.plugin.SystemPlugin/GetMetadata";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<common_pb.Empty>;
    requestDeserialize: grpc.deserialize<common_pb.Empty>;
    responseSerialize: grpc.serialize<plugin_pb.Metadata>;
    responseDeserialize: grpc.deserialize<plugin_pb.Metadata>;
}

export const SystemPluginService: ISystemPluginService;

export interface ISystemPluginServer {
    getMetadata: grpc.handleUnaryCall<common_pb.Empty, plugin_pb.Metadata>;
}

export interface ISystemPluginClient {
    getMetadata(request: common_pb.Empty, callback: (error: grpc.ServiceError | null, response: plugin_pb.Metadata) => void): grpc.ClientUnaryCall;
    getMetadata(request: common_pb.Empty, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: plugin_pb.Metadata) => void): grpc.ClientUnaryCall;
    getMetadata(request: common_pb.Empty, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: plugin_pb.Metadata) => void): grpc.ClientUnaryCall;
}

export class SystemPluginClient extends grpc.Client implements ISystemPluginClient {
    constructor(address: string, credentials: grpc.ChannelCredentials, options?: object);
    public getMetadata(request: common_pb.Empty, callback: (error: grpc.ServiceError | null, response: plugin_pb.Metadata) => void): grpc.ClientUnaryCall;
    public getMetadata(request: common_pb.Empty, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: plugin_pb.Metadata) => void): grpc.ClientUnaryCall;
    public getMetadata(request: common_pb.Empty, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: plugin_pb.Metadata) => void): grpc.ClientUnaryCall;
}
