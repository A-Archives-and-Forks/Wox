// package: wox.plugin
// file: api.proto

/* tslint:disable */
/* eslint-disable */

import * as jspb from "google-protobuf";
import * as common_pb from "./common_pb";

export class ChangeQueryRequest extends jspb.Message { 
    getQueryType(): ChangeQueryRequest.QueryType;
    setQueryType(value: ChangeQueryRequest.QueryType): ChangeQueryRequest;
    getQueryText(): string;
    setQueryText(value: string): ChangeQueryRequest;

    hasQuerySelection(): boolean;
    clearQuerySelection(): void;
    getQuerySelection(): common_pb.Selection | undefined;
    setQuerySelection(value?: common_pb.Selection): ChangeQueryRequest;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): ChangeQueryRequest.AsObject;
    static toObject(includeInstance: boolean, msg: ChangeQueryRequest): ChangeQueryRequest.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: ChangeQueryRequest, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): ChangeQueryRequest;
    static deserializeBinaryFromReader(message: ChangeQueryRequest, reader: jspb.BinaryReader): ChangeQueryRequest;
}

export namespace ChangeQueryRequest {
    export type AsObject = {
        queryType: ChangeQueryRequest.QueryType,
        queryText: string,
        querySelection?: common_pb.Selection.AsObject,
    }

    export enum QueryType {
    INPUT = 0,
    SELECTION = 1,
    }

}

export class NotifyRequest extends jspb.Message { 
    getMessage(): string;
    setMessage(value: string): NotifyRequest;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): NotifyRequest.AsObject;
    static toObject(includeInstance: boolean, msg: NotifyRequest): NotifyRequest.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: NotifyRequest, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): NotifyRequest;
    static deserializeBinaryFromReader(message: NotifyRequest, reader: jspb.BinaryReader): NotifyRequest;
}

export namespace NotifyRequest {
    export type AsObject = {
        message: string,
    }
}

export class LogRequest extends jspb.Message { 
    getLevel(): LogRequest.LogLevel;
    setLevel(value: LogRequest.LogLevel): LogRequest;
    getMessage(): string;
    setMessage(value: string): LogRequest;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): LogRequest.AsObject;
    static toObject(includeInstance: boolean, msg: LogRequest): LogRequest.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: LogRequest, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): LogRequest;
    static deserializeBinaryFromReader(message: LogRequest, reader: jspb.BinaryReader): LogRequest;
}

export namespace LogRequest {
    export type AsObject = {
        level: LogRequest.LogLevel,
        message: string,
    }

    export enum LogLevel {
    INFO = 0,
    ERROR = 1,
    DEBUG = 2,
    WARNING = 3,
    }

}

export class TranslationRequest extends jspb.Message { 
    getKey(): string;
    setKey(value: string): TranslationRequest;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): TranslationRequest.AsObject;
    static toObject(includeInstance: boolean, msg: TranslationRequest): TranslationRequest.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: TranslationRequest, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): TranslationRequest;
    static deserializeBinaryFromReader(message: TranslationRequest, reader: jspb.BinaryReader): TranslationRequest;
}

export namespace TranslationRequest {
    export type AsObject = {
        key: string,
    }
}

export class TranslationResponse extends jspb.Message { 
    getText(): string;
    setText(value: string): TranslationResponse;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): TranslationResponse.AsObject;
    static toObject(includeInstance: boolean, msg: TranslationResponse): TranslationResponse.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: TranslationResponse, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): TranslationResponse;
    static deserializeBinaryFromReader(message: TranslationResponse, reader: jspb.BinaryReader): TranslationResponse;
}

export namespace TranslationResponse {
    export type AsObject = {
        text: string,
    }
}

export class SettingRequest extends jspb.Message { 
    getKey(): string;
    setKey(value: string): SettingRequest;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): SettingRequest.AsObject;
    static toObject(includeInstance: boolean, msg: SettingRequest): SettingRequest.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: SettingRequest, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): SettingRequest;
    static deserializeBinaryFromReader(message: SettingRequest, reader: jspb.BinaryReader): SettingRequest;
}

export namespace SettingRequest {
    export type AsObject = {
        key: string,
    }
}

export class SettingResponse extends jspb.Message { 
    getValue(): string;
    setValue(value: string): SettingResponse;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): SettingResponse.AsObject;
    static toObject(includeInstance: boolean, msg: SettingResponse): SettingResponse.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: SettingResponse, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): SettingResponse;
    static deserializeBinaryFromReader(message: SettingResponse, reader: jspb.BinaryReader): SettingResponse;
}

export namespace SettingResponse {
    export type AsObject = {
        value: string,
    }
}

export class SaveSettingRequest extends jspb.Message { 
    getKey(): string;
    setKey(value: string): SaveSettingRequest;
    getValue(): string;
    setValue(value: string): SaveSettingRequest;
    getIsPlatformSpecific(): boolean;
    setIsPlatformSpecific(value: boolean): SaveSettingRequest;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): SaveSettingRequest.AsObject;
    static toObject(includeInstance: boolean, msg: SaveSettingRequest): SaveSettingRequest.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: SaveSettingRequest, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): SaveSettingRequest;
    static deserializeBinaryFromReader(message: SaveSettingRequest, reader: jspb.BinaryReader): SaveSettingRequest;
}

export namespace SaveSettingRequest {
    export type AsObject = {
        key: string,
        value: string,
        isPlatformSpecific: boolean,
    }
}

export class RegisterQueryCommandsRequest extends jspb.Message { 
    clearCommandsList(): void;
    getCommandsList(): Array<common_pb.MetadataCommand>;
    setCommandsList(value: Array<common_pb.MetadataCommand>): RegisterQueryCommandsRequest;
    addCommands(value?: common_pb.MetadataCommand, index?: number): common_pb.MetadataCommand;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): RegisterQueryCommandsRequest.AsObject;
    static toObject(includeInstance: boolean, msg: RegisterQueryCommandsRequest): RegisterQueryCommandsRequest.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: RegisterQueryCommandsRequest, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): RegisterQueryCommandsRequest;
    static deserializeBinaryFromReader(message: RegisterQueryCommandsRequest, reader: jspb.BinaryReader): RegisterQueryCommandsRequest;
}

export namespace RegisterQueryCommandsRequest {
    export type AsObject = {
        commandsList: Array<common_pb.MetadataCommand.AsObject>,
    }
}

export class AIChatRequest extends jspb.Message { 
    getModel(): string;
    setModel(value: string): AIChatRequest;
    clearConversationsList(): void;
    getConversationsList(): Array<Conversation>;
    setConversationsList(value: Array<Conversation>): AIChatRequest;
    addConversations(value?: Conversation, index?: number): Conversation;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): AIChatRequest.AsObject;
    static toObject(includeInstance: boolean, msg: AIChatRequest): AIChatRequest.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: AIChatRequest, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): AIChatRequest;
    static deserializeBinaryFromReader(message: AIChatRequest, reader: jspb.BinaryReader): AIChatRequest;
}

export namespace AIChatRequest {
    export type AsObject = {
        model: string,
        conversationsList: Array<Conversation.AsObject>,
    }
}

export class Conversation extends jspb.Message { 
    getRole(): Conversation.Role;
    setRole(value: Conversation.Role): Conversation;
    getContent(): string;
    setContent(value: string): Conversation;
    clearImagesList(): void;
    getImagesList(): Array<Uint8Array | string>;
    getImagesList_asU8(): Array<Uint8Array>;
    getImagesList_asB64(): Array<string>;
    setImagesList(value: Array<Uint8Array | string>): Conversation;
    addImages(value: Uint8Array | string, index?: number): Uint8Array | string;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): Conversation.AsObject;
    static toObject(includeInstance: boolean, msg: Conversation): Conversation.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: Conversation, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): Conversation;
    static deserializeBinaryFromReader(message: Conversation, reader: jspb.BinaryReader): Conversation;
}

export namespace Conversation {
    export type AsObject = {
        role: Conversation.Role,
        content: string,
        imagesList: Array<Uint8Array | string>,
    }

    export enum Role {
    USER = 0,
    SYSTEM = 1,
    }

}

export class AIChatResponse extends jspb.Message { 
    getType(): AIChatResponse.Type;
    setType(value: AIChatResponse.Type): AIChatResponse;
    getContent(): string;
    setContent(value: string): AIChatResponse;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): AIChatResponse.AsObject;
    static toObject(includeInstance: boolean, msg: AIChatResponse): AIChatResponse.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: AIChatResponse, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): AIChatResponse;
    static deserializeBinaryFromReader(message: AIChatResponse, reader: jspb.BinaryReader): AIChatResponse;
}

export namespace AIChatResponse {
    export type AsObject = {
        type: AIChatResponse.Type,
        content: string,
    }

    export enum Type {
    STREAMING = 0,
    FINISHED = 1,
    ERROR = 2,
    }

}

export class SettingChangedEvent extends jspb.Message { 
    getKey(): string;
    setKey(value: string): SettingChangedEvent;
    getValue(): string;
    setValue(value: string): SettingChangedEvent;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): SettingChangedEvent.AsObject;
    static toObject(includeInstance: boolean, msg: SettingChangedEvent): SettingChangedEvent.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: SettingChangedEvent, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): SettingChangedEvent;
    static deserializeBinaryFromReader(message: SettingChangedEvent, reader: jspb.BinaryReader): SettingChangedEvent;
}

export namespace SettingChangedEvent {
    export type AsObject = {
        key: string,
        value: string,
    }
}

export class DynamicSettingEvent extends jspb.Message { 
    getKey(): string;
    setKey(value: string): DynamicSettingEvent;

    hasSetting(): boolean;
    clearSetting(): void;
    getSetting(): common_pb.MetadataSetting | undefined;
    setSetting(value?: common_pb.MetadataSetting): DynamicSettingEvent;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): DynamicSettingEvent.AsObject;
    static toObject(includeInstance: boolean, msg: DynamicSettingEvent): DynamicSettingEvent.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: DynamicSettingEvent, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): DynamicSettingEvent;
    static deserializeBinaryFromReader(message: DynamicSettingEvent, reader: jspb.BinaryReader): DynamicSettingEvent;
}

export namespace DynamicSettingEvent {
    export type AsObject = {
        key: string,
        setting?: common_pb.MetadataSetting.AsObject,
    }
}

export class DeepLinkEvent extends jspb.Message { 

    hasArguments(): boolean;
    clearArguments(): void;
    getArguments(): common_pb.MapString | undefined;
    setArguments(value?: common_pb.MapString): DeepLinkEvent;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): DeepLinkEvent.AsObject;
    static toObject(includeInstance: boolean, msg: DeepLinkEvent): DeepLinkEvent.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: DeepLinkEvent, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): DeepLinkEvent;
    static deserializeBinaryFromReader(message: DeepLinkEvent, reader: jspb.BinaryReader): DeepLinkEvent;
}

export namespace DeepLinkEvent {
    export type AsObject = {
        arguments?: common_pb.MapString.AsObject,
    }
}

export class UnloadEvent extends jspb.Message { 

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): UnloadEvent.AsObject;
    static toObject(includeInstance: boolean, msg: UnloadEvent): UnloadEvent.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: UnloadEvent, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): UnloadEvent;
    static deserializeBinaryFromReader(message: UnloadEvent, reader: jspb.BinaryReader): UnloadEvent;
}

export namespace UnloadEvent {
    export type AsObject = {
    }
}
