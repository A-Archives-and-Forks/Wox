// package: wox.plugin
// file: plugin.proto

/* tslint:disable */
/* eslint-disable */

import * as jspb from "google-protobuf";
import * as common_pb from "./common_pb";
import * as api_pb from "./api_pb";

export class PluginInitContext extends jspb.Message { 

    hasContext(): boolean;
    clearContext(): void;
    getContext(): Context | undefined;
    setContext(value?: Context): PluginInitContext;

    hasParams(): boolean;
    clearParams(): void;
    getParams(): PluginInitParams | undefined;
    setParams(value?: PluginInitParams): PluginInitContext;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): PluginInitContext.AsObject;
    static toObject(includeInstance: boolean, msg: PluginInitContext): PluginInitContext.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: PluginInitContext, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): PluginInitContext;
    static deserializeBinaryFromReader(message: PluginInitContext, reader: jspb.BinaryReader): PluginInitContext;
}

export namespace PluginInitContext {
    export type AsObject = {
        context?: Context.AsObject,
        params?: PluginInitParams.AsObject,
    }
}

export class PluginQueryContext extends jspb.Message { 

    hasContext(): boolean;
    clearContext(): void;
    getContext(): Context | undefined;
    setContext(value?: Context): PluginQueryContext;

    hasQuery(): boolean;
    clearQuery(): void;
    getQuery(): Query | undefined;
    setQuery(value?: Query): PluginQueryContext;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): PluginQueryContext.AsObject;
    static toObject(includeInstance: boolean, msg: PluginQueryContext): PluginQueryContext.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: PluginQueryContext, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): PluginQueryContext;
    static deserializeBinaryFromReader(message: PluginQueryContext, reader: jspb.BinaryReader): PluginQueryContext;
}

export namespace PluginQueryContext {
    export type AsObject = {
        context?: Context.AsObject,
        query?: Query.AsObject,
    }
}

export class Context extends jspb.Message { 
    getTraceId(): string;
    setTraceId(value: string): Context;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): Context.AsObject;
    static toObject(includeInstance: boolean, msg: Context): Context.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: Context, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): Context;
    static deserializeBinaryFromReader(message: Context, reader: jspb.BinaryReader): Context;
}

export namespace Context {
    export type AsObject = {
        traceId: string,
    }
}

export class PluginInitParams extends jspb.Message { 

    hasApi(): boolean;
    clearApi(): void;
    getApi(): PublicAPIClient | undefined;
    setApi(value?: PublicAPIClient): PluginInitParams;
    getPluginDirectory(): string;
    setPluginDirectory(value: string): PluginInitParams;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): PluginInitParams.AsObject;
    static toObject(includeInstance: boolean, msg: PluginInitParams): PluginInitParams.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: PluginInitParams, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): PluginInitParams;
    static deserializeBinaryFromReader(message: PluginInitParams, reader: jspb.BinaryReader): PluginInitParams;
}

export namespace PluginInitParams {
    export type AsObject = {
        api?: PublicAPIClient.AsObject,
        pluginDirectory: string,
    }
}

export class PublicAPIClient extends jspb.Message { 
    getClientId(): string;
    setClientId(value: string): PublicAPIClient;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): PublicAPIClient.AsObject;
    static toObject(includeInstance: boolean, msg: PublicAPIClient): PublicAPIClient.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: PublicAPIClient, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): PublicAPIClient;
    static deserializeBinaryFromReader(message: PublicAPIClient, reader: jspb.BinaryReader): PublicAPIClient;
}

export namespace PublicAPIClient {
    export type AsObject = {
        clientId: string,
    }
}

export class Query extends jspb.Message { 
    getType(): Query.QueryType;
    setType(value: Query.QueryType): Query;
    getRawQuery(): string;
    setRawQuery(value: string): Query;
    getTriggerKeyword(): string;
    setTriggerKeyword(value: string): Query;
    getCommand(): string;
    setCommand(value: string): Query;
    getSearch(): string;
    setSearch(value: string): Query;

    hasSelection(): boolean;
    clearSelection(): void;
    getSelection(): common_pb.Selection | undefined;
    setSelection(value?: common_pb.Selection): Query;

    hasEnv(): boolean;
    clearEnv(): void;
    getEnv(): common_pb.QueryEnv | undefined;
    setEnv(value?: common_pb.QueryEnv): Query;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): Query.AsObject;
    static toObject(includeInstance: boolean, msg: Query): Query.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: Query, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): Query;
    static deserializeBinaryFromReader(message: Query, reader: jspb.BinaryReader): Query;
}

export namespace Query {
    export type AsObject = {
        type: Query.QueryType,
        rawQuery: string,
        triggerKeyword: string,
        command: string,
        search: string,
        selection?: common_pb.Selection.AsObject,
        env?: common_pb.QueryEnv.AsObject,
    }

    export enum QueryType {
    INPUT = 0,
    SELECTION = 1,
    }

}

export class QueryResponse extends jspb.Message { 
    clearResultsList(): void;
    getResultsList(): Array<Result>;
    setResultsList(value: Array<Result>): QueryResponse;
    addResults(value?: Result, index?: number): Result;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): QueryResponse.AsObject;
    static toObject(includeInstance: boolean, msg: QueryResponse): QueryResponse.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: QueryResponse, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): QueryResponse;
    static deserializeBinaryFromReader(message: QueryResponse, reader: jspb.BinaryReader): QueryResponse;
}

export namespace QueryResponse {
    export type AsObject = {
        resultsList: Array<Result.AsObject>,
    }
}

export class Result extends jspb.Message { 
    getId(): string;
    setId(value: string): Result;
    getTitle(): string;
    setTitle(value: string): Result;
    getSubtitle(): string;
    setSubtitle(value: string): Result;

    hasIcon(): boolean;
    clearIcon(): void;
    getIcon(): common_pb.WoxImage | undefined;
    setIcon(value?: common_pb.WoxImage): Result;

    hasPreview(): boolean;
    clearPreview(): void;
    getPreview(): common_pb.WoxPreview | undefined;
    setPreview(value?: common_pb.WoxPreview): Result;
    getScore(): number;
    setScore(value: number): Result;
    getGroup(): string;
    setGroup(value: string): Result;
    getGroupScore(): number;
    setGroupScore(value: number): Result;
    clearTailsList(): void;
    getTailsList(): Array<common_pb.ResultTail>;
    setTailsList(value: Array<common_pb.ResultTail>): Result;
    addTails(value?: common_pb.ResultTail, index?: number): common_pb.ResultTail;
    getContextData(): string;
    setContextData(value: string): Result;
    clearActionsList(): void;
    getActionsList(): Array<common_pb.ResultAction>;
    setActionsList(value: Array<common_pb.ResultAction>): Result;
    addActions(value?: common_pb.ResultAction, index?: number): common_pb.ResultAction;
    getRefreshInterval(): number;
    setRefreshInterval(value: number): Result;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): Result.AsObject;
    static toObject(includeInstance: boolean, msg: Result): Result.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: Result, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): Result;
    static deserializeBinaryFromReader(message: Result, reader: jspb.BinaryReader): Result;
}

export namespace Result {
    export type AsObject = {
        id: string,
        title: string,
        subtitle: string,
        icon?: common_pb.WoxImage.AsObject,
        preview?: common_pb.WoxPreview.AsObject,
        score: number,
        group: string,
        groupScore: number,
        tailsList: Array<common_pb.ResultTail.AsObject>,
        contextData: string,
        actionsList: Array<common_pb.ResultAction.AsObject>,
        refreshInterval: number,
    }
}

export class Metadata extends jspb.Message { 
    getId(): string;
    setId(value: string): Metadata;
    getName(): string;
    setName(value: string): Metadata;
    getDescription(): string;
    setDescription(value: string): Metadata;
    getAuthor(): string;
    setAuthor(value: string): Metadata;
    getVersion(): string;
    setVersion(value: string): Metadata;
    getWebsite(): string;
    setWebsite(value: string): Metadata;
    getIcon(): string;
    setIcon(value: string): Metadata;
    clearSupportedFeaturesList(): void;
    getSupportedFeaturesList(): Array<string>;
    setSupportedFeaturesList(value: Array<string>): Metadata;
    addSupportedFeatures(value: string, index?: number): string;
    clearCommandsList(): void;
    getCommandsList(): Array<common_pb.MetadataCommand>;
    setCommandsList(value: Array<common_pb.MetadataCommand>): Metadata;
    addCommands(value?: common_pb.MetadataCommand, index?: number): common_pb.MetadataCommand;
    clearSettingsList(): void;
    getSettingsList(): Array<common_pb.MetadataSetting>;
    setSettingsList(value: Array<common_pb.MetadataSetting>): Metadata;
    addSettings(value?: common_pb.MetadataSetting, index?: number): common_pb.MetadataSetting;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): Metadata.AsObject;
    static toObject(includeInstance: boolean, msg: Metadata): Metadata.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: Metadata, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): Metadata;
    static deserializeBinaryFromReader(message: Metadata, reader: jspb.BinaryReader): Metadata;
}

export namespace Metadata {
    export type AsObject = {
        id: string,
        name: string,
        description: string,
        author: string,
        version: string,
        website: string,
        icon: string,
        supportedFeaturesList: Array<string>,
        commandsList: Array<common_pb.MetadataCommand.AsObject>,
        settingsList: Array<common_pb.MetadataSetting.AsObject>,
    }
}
