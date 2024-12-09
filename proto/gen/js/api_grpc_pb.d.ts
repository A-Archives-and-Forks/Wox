// package: wox.plugin
// file: api.proto

/* tslint:disable */
/* eslint-disable */

import * as grpc from "grpc";
import * as api_pb from "./api_pb";
import * as common_pb from "./common_pb";

interface IPublicAPIService extends grpc.ServiceDefinition<grpc.UntypedServiceImplementation> {
    changeQuery: IPublicAPIService_IChangeQuery;
    hideApp: IPublicAPIService_IHideApp;
    showApp: IPublicAPIService_IShowApp;
    notify: IPublicAPIService_INotify;
    log: IPublicAPIService_ILog;
    getTranslation: IPublicAPIService_IGetTranslation;
    getSetting: IPublicAPIService_IGetSetting;
    saveSetting: IPublicAPIService_ISaveSetting;
    registerQueryCommands: IPublicAPIService_IRegisterQueryCommands;
    aIChatStream: IPublicAPIService_IAIChatStream;
    onSettingChanged: IPublicAPIService_IOnSettingChanged;
    onGetDynamicSetting: IPublicAPIService_IOnGetDynamicSetting;
    onDeepLink: IPublicAPIService_IOnDeepLink;
    onUnload: IPublicAPIService_IOnUnload;
}

interface IPublicAPIService_IChangeQuery extends grpc.MethodDefinition<api_pb.ChangeQueryRequest, common_pb.Empty> {
    path: "/wox.plugin.PublicAPI/ChangeQuery";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<api_pb.ChangeQueryRequest>;
    requestDeserialize: grpc.deserialize<api_pb.ChangeQueryRequest>;
    responseSerialize: grpc.serialize<common_pb.Empty>;
    responseDeserialize: grpc.deserialize<common_pb.Empty>;
}
interface IPublicAPIService_IHideApp extends grpc.MethodDefinition<common_pb.Empty, common_pb.Empty> {
    path: "/wox.plugin.PublicAPI/HideApp";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<common_pb.Empty>;
    requestDeserialize: grpc.deserialize<common_pb.Empty>;
    responseSerialize: grpc.serialize<common_pb.Empty>;
    responseDeserialize: grpc.deserialize<common_pb.Empty>;
}
interface IPublicAPIService_IShowApp extends grpc.MethodDefinition<common_pb.Empty, common_pb.Empty> {
    path: "/wox.plugin.PublicAPI/ShowApp";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<common_pb.Empty>;
    requestDeserialize: grpc.deserialize<common_pb.Empty>;
    responseSerialize: grpc.serialize<common_pb.Empty>;
    responseDeserialize: grpc.deserialize<common_pb.Empty>;
}
interface IPublicAPIService_INotify extends grpc.MethodDefinition<api_pb.NotifyRequest, common_pb.Empty> {
    path: "/wox.plugin.PublicAPI/Notify";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<api_pb.NotifyRequest>;
    requestDeserialize: grpc.deserialize<api_pb.NotifyRequest>;
    responseSerialize: grpc.serialize<common_pb.Empty>;
    responseDeserialize: grpc.deserialize<common_pb.Empty>;
}
interface IPublicAPIService_ILog extends grpc.MethodDefinition<api_pb.LogRequest, common_pb.Empty> {
    path: "/wox.plugin.PublicAPI/Log";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<api_pb.LogRequest>;
    requestDeserialize: grpc.deserialize<api_pb.LogRequest>;
    responseSerialize: grpc.serialize<common_pb.Empty>;
    responseDeserialize: grpc.deserialize<common_pb.Empty>;
}
interface IPublicAPIService_IGetTranslation extends grpc.MethodDefinition<api_pb.TranslationRequest, api_pb.TranslationResponse> {
    path: "/wox.plugin.PublicAPI/GetTranslation";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<api_pb.TranslationRequest>;
    requestDeserialize: grpc.deserialize<api_pb.TranslationRequest>;
    responseSerialize: grpc.serialize<api_pb.TranslationResponse>;
    responseDeserialize: grpc.deserialize<api_pb.TranslationResponse>;
}
interface IPublicAPIService_IGetSetting extends grpc.MethodDefinition<api_pb.SettingRequest, api_pb.SettingResponse> {
    path: "/wox.plugin.PublicAPI/GetSetting";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<api_pb.SettingRequest>;
    requestDeserialize: grpc.deserialize<api_pb.SettingRequest>;
    responseSerialize: grpc.serialize<api_pb.SettingResponse>;
    responseDeserialize: grpc.deserialize<api_pb.SettingResponse>;
}
interface IPublicAPIService_ISaveSetting extends grpc.MethodDefinition<api_pb.SaveSettingRequest, common_pb.Empty> {
    path: "/wox.plugin.PublicAPI/SaveSetting";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<api_pb.SaveSettingRequest>;
    requestDeserialize: grpc.deserialize<api_pb.SaveSettingRequest>;
    responseSerialize: grpc.serialize<common_pb.Empty>;
    responseDeserialize: grpc.deserialize<common_pb.Empty>;
}
interface IPublicAPIService_IRegisterQueryCommands extends grpc.MethodDefinition<api_pb.RegisterQueryCommandsRequest, common_pb.Empty> {
    path: "/wox.plugin.PublicAPI/RegisterQueryCommands";
    requestStream: false;
    responseStream: false;
    requestSerialize: grpc.serialize<api_pb.RegisterQueryCommandsRequest>;
    requestDeserialize: grpc.deserialize<api_pb.RegisterQueryCommandsRequest>;
    responseSerialize: grpc.serialize<common_pb.Empty>;
    responseDeserialize: grpc.deserialize<common_pb.Empty>;
}
interface IPublicAPIService_IAIChatStream extends grpc.MethodDefinition<api_pb.AIChatRequest, api_pb.AIChatResponse> {
    path: "/wox.plugin.PublicAPI/AIChatStream";
    requestStream: false;
    responseStream: true;
    requestSerialize: grpc.serialize<api_pb.AIChatRequest>;
    requestDeserialize: grpc.deserialize<api_pb.AIChatRequest>;
    responseSerialize: grpc.serialize<api_pb.AIChatResponse>;
    responseDeserialize: grpc.deserialize<api_pb.AIChatResponse>;
}
interface IPublicAPIService_IOnSettingChanged extends grpc.MethodDefinition<common_pb.Empty, api_pb.SettingChangedEvent> {
    path: "/wox.plugin.PublicAPI/OnSettingChanged";
    requestStream: false;
    responseStream: true;
    requestSerialize: grpc.serialize<common_pb.Empty>;
    requestDeserialize: grpc.deserialize<common_pb.Empty>;
    responseSerialize: grpc.serialize<api_pb.SettingChangedEvent>;
    responseDeserialize: grpc.deserialize<api_pb.SettingChangedEvent>;
}
interface IPublicAPIService_IOnGetDynamicSetting extends grpc.MethodDefinition<common_pb.Empty, api_pb.DynamicSettingEvent> {
    path: "/wox.plugin.PublicAPI/OnGetDynamicSetting";
    requestStream: false;
    responseStream: true;
    requestSerialize: grpc.serialize<common_pb.Empty>;
    requestDeserialize: grpc.deserialize<common_pb.Empty>;
    responseSerialize: grpc.serialize<api_pb.DynamicSettingEvent>;
    responseDeserialize: grpc.deserialize<api_pb.DynamicSettingEvent>;
}
interface IPublicAPIService_IOnDeepLink extends grpc.MethodDefinition<common_pb.Empty, api_pb.DeepLinkEvent> {
    path: "/wox.plugin.PublicAPI/OnDeepLink";
    requestStream: false;
    responseStream: true;
    requestSerialize: grpc.serialize<common_pb.Empty>;
    requestDeserialize: grpc.deserialize<common_pb.Empty>;
    responseSerialize: grpc.serialize<api_pb.DeepLinkEvent>;
    responseDeserialize: grpc.deserialize<api_pb.DeepLinkEvent>;
}
interface IPublicAPIService_IOnUnload extends grpc.MethodDefinition<common_pb.Empty, api_pb.UnloadEvent> {
    path: "/wox.plugin.PublicAPI/OnUnload";
    requestStream: false;
    responseStream: true;
    requestSerialize: grpc.serialize<common_pb.Empty>;
    requestDeserialize: grpc.deserialize<common_pb.Empty>;
    responseSerialize: grpc.serialize<api_pb.UnloadEvent>;
    responseDeserialize: grpc.deserialize<api_pb.UnloadEvent>;
}

export const PublicAPIService: IPublicAPIService;

export interface IPublicAPIServer {
    changeQuery: grpc.handleUnaryCall<api_pb.ChangeQueryRequest, common_pb.Empty>;
    hideApp: grpc.handleUnaryCall<common_pb.Empty, common_pb.Empty>;
    showApp: grpc.handleUnaryCall<common_pb.Empty, common_pb.Empty>;
    notify: grpc.handleUnaryCall<api_pb.NotifyRequest, common_pb.Empty>;
    log: grpc.handleUnaryCall<api_pb.LogRequest, common_pb.Empty>;
    getTranslation: grpc.handleUnaryCall<api_pb.TranslationRequest, api_pb.TranslationResponse>;
    getSetting: grpc.handleUnaryCall<api_pb.SettingRequest, api_pb.SettingResponse>;
    saveSetting: grpc.handleUnaryCall<api_pb.SaveSettingRequest, common_pb.Empty>;
    registerQueryCommands: grpc.handleUnaryCall<api_pb.RegisterQueryCommandsRequest, common_pb.Empty>;
    aIChatStream: grpc.handleServerStreamingCall<api_pb.AIChatRequest, api_pb.AIChatResponse>;
    onSettingChanged: grpc.handleServerStreamingCall<common_pb.Empty, api_pb.SettingChangedEvent>;
    onGetDynamicSetting: grpc.handleServerStreamingCall<common_pb.Empty, api_pb.DynamicSettingEvent>;
    onDeepLink: grpc.handleServerStreamingCall<common_pb.Empty, api_pb.DeepLinkEvent>;
    onUnload: grpc.handleServerStreamingCall<common_pb.Empty, api_pb.UnloadEvent>;
}

export interface IPublicAPIClient {
    changeQuery(request: api_pb.ChangeQueryRequest, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    changeQuery(request: api_pb.ChangeQueryRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    changeQuery(request: api_pb.ChangeQueryRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    hideApp(request: common_pb.Empty, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    hideApp(request: common_pb.Empty, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    hideApp(request: common_pb.Empty, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    showApp(request: common_pb.Empty, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    showApp(request: common_pb.Empty, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    showApp(request: common_pb.Empty, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    notify(request: api_pb.NotifyRequest, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    notify(request: api_pb.NotifyRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    notify(request: api_pb.NotifyRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    log(request: api_pb.LogRequest, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    log(request: api_pb.LogRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    log(request: api_pb.LogRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    getTranslation(request: api_pb.TranslationRequest, callback: (error: grpc.ServiceError | null, response: api_pb.TranslationResponse) => void): grpc.ClientUnaryCall;
    getTranslation(request: api_pb.TranslationRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: api_pb.TranslationResponse) => void): grpc.ClientUnaryCall;
    getTranslation(request: api_pb.TranslationRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: api_pb.TranslationResponse) => void): grpc.ClientUnaryCall;
    getSetting(request: api_pb.SettingRequest, callback: (error: grpc.ServiceError | null, response: api_pb.SettingResponse) => void): grpc.ClientUnaryCall;
    getSetting(request: api_pb.SettingRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: api_pb.SettingResponse) => void): grpc.ClientUnaryCall;
    getSetting(request: api_pb.SettingRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: api_pb.SettingResponse) => void): grpc.ClientUnaryCall;
    saveSetting(request: api_pb.SaveSettingRequest, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    saveSetting(request: api_pb.SaveSettingRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    saveSetting(request: api_pb.SaveSettingRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    registerQueryCommands(request: api_pb.RegisterQueryCommandsRequest, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    registerQueryCommands(request: api_pb.RegisterQueryCommandsRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    registerQueryCommands(request: api_pb.RegisterQueryCommandsRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    aIChatStream(request: api_pb.AIChatRequest, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.AIChatResponse>;
    aIChatStream(request: api_pb.AIChatRequest, metadata?: grpc.Metadata, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.AIChatResponse>;
    onSettingChanged(request: common_pb.Empty, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.SettingChangedEvent>;
    onSettingChanged(request: common_pb.Empty, metadata?: grpc.Metadata, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.SettingChangedEvent>;
    onGetDynamicSetting(request: common_pb.Empty, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.DynamicSettingEvent>;
    onGetDynamicSetting(request: common_pb.Empty, metadata?: grpc.Metadata, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.DynamicSettingEvent>;
    onDeepLink(request: common_pb.Empty, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.DeepLinkEvent>;
    onDeepLink(request: common_pb.Empty, metadata?: grpc.Metadata, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.DeepLinkEvent>;
    onUnload(request: common_pb.Empty, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.UnloadEvent>;
    onUnload(request: common_pb.Empty, metadata?: grpc.Metadata, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.UnloadEvent>;
}

export class PublicAPIClient extends grpc.Client implements IPublicAPIClient {
    constructor(address: string, credentials: grpc.ChannelCredentials, options?: object);
    public changeQuery(request: api_pb.ChangeQueryRequest, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public changeQuery(request: api_pb.ChangeQueryRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public changeQuery(request: api_pb.ChangeQueryRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public hideApp(request: common_pb.Empty, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public hideApp(request: common_pb.Empty, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public hideApp(request: common_pb.Empty, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public showApp(request: common_pb.Empty, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public showApp(request: common_pb.Empty, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public showApp(request: common_pb.Empty, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public notify(request: api_pb.NotifyRequest, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public notify(request: api_pb.NotifyRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public notify(request: api_pb.NotifyRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public log(request: api_pb.LogRequest, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public log(request: api_pb.LogRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public log(request: api_pb.LogRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public getTranslation(request: api_pb.TranslationRequest, callback: (error: grpc.ServiceError | null, response: api_pb.TranslationResponse) => void): grpc.ClientUnaryCall;
    public getTranslation(request: api_pb.TranslationRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: api_pb.TranslationResponse) => void): grpc.ClientUnaryCall;
    public getTranslation(request: api_pb.TranslationRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: api_pb.TranslationResponse) => void): grpc.ClientUnaryCall;
    public getSetting(request: api_pb.SettingRequest, callback: (error: grpc.ServiceError | null, response: api_pb.SettingResponse) => void): grpc.ClientUnaryCall;
    public getSetting(request: api_pb.SettingRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: api_pb.SettingResponse) => void): grpc.ClientUnaryCall;
    public getSetting(request: api_pb.SettingRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: api_pb.SettingResponse) => void): grpc.ClientUnaryCall;
    public saveSetting(request: api_pb.SaveSettingRequest, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public saveSetting(request: api_pb.SaveSettingRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public saveSetting(request: api_pb.SaveSettingRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public registerQueryCommands(request: api_pb.RegisterQueryCommandsRequest, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public registerQueryCommands(request: api_pb.RegisterQueryCommandsRequest, metadata: grpc.Metadata, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public registerQueryCommands(request: api_pb.RegisterQueryCommandsRequest, metadata: grpc.Metadata, options: Partial<grpc.CallOptions>, callback: (error: grpc.ServiceError | null, response: common_pb.Empty) => void): grpc.ClientUnaryCall;
    public aIChatStream(request: api_pb.AIChatRequest, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.AIChatResponse>;
    public aIChatStream(request: api_pb.AIChatRequest, metadata?: grpc.Metadata, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.AIChatResponse>;
    public onSettingChanged(request: common_pb.Empty, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.SettingChangedEvent>;
    public onSettingChanged(request: common_pb.Empty, metadata?: grpc.Metadata, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.SettingChangedEvent>;
    public onGetDynamicSetting(request: common_pb.Empty, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.DynamicSettingEvent>;
    public onGetDynamicSetting(request: common_pb.Empty, metadata?: grpc.Metadata, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.DynamicSettingEvent>;
    public onDeepLink(request: common_pb.Empty, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.DeepLinkEvent>;
    public onDeepLink(request: common_pb.Empty, metadata?: grpc.Metadata, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.DeepLinkEvent>;
    public onUnload(request: common_pb.Empty, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.UnloadEvent>;
    public onUnload(request: common_pb.Empty, metadata?: grpc.Metadata, options?: Partial<grpc.CallOptions>): grpc.ClientReadableStream<api_pb.UnloadEvent>;
}
