import { ChangeQueryParam, PublicAPI } from "@wox-launcher/wox-plugin"
import { WebSocket } from "ws"
import { PluginJsonRpcRequest, PluginJsonRpcTypeRequest } from "./jsonrpc"
import * as crypto from "crypto"
import { waitingForResponse } from "./index"
import Deferred from "promise-deferred"
import { logger } from "./logger"

export class PluginAPI implements PublicAPI {
  ws: WebSocket
  pluginId: string
  pluginName: string
  settingChangeCallbacks: Map<string, (key: string, value: string) => void>

  constructor(ws: WebSocket, pluginId: string, pluginName: string) {
    this.ws = ws
    this.pluginId = pluginId
    this.pluginName = pluginName
    this.settingChangeCallbacks = new Map<string, (key: string, value: string) => void>()
  }

  async invokeMethod(method: string, params: { [key: string]: string }): Promise<unknown> {
    const startTime = Date.now()
    const requestId = crypto.randomUUID()

    logger.info(`[${this.pluginName}] start invoke method to Wox: ${method}, id: ${requestId} parameters: ${JSON.stringify(params)}`)

    this.ws.send(
      JSON.stringify({
        Id: requestId,
        Method: method,
        Type: PluginJsonRpcTypeRequest,
        Params: params,
        PluginId: this.pluginId,
        PluginName: this.pluginName
      } as PluginJsonRpcRequest)
    )
    const deferred = new Deferred<unknown>()
    waitingForResponse[requestId] = deferred

    const result = await deferred.promise
    const endTime = Date.now()
    logger.info(`[${this.pluginName}] invoke method to Wox finished: ${method}, time: ${endTime - startTime}ms`)
    return result
  }

  async ChangeQuery(query: ChangeQueryParam): Promise<void> {
    await this.invokeMethod("ChangeQuery", {
      queryType: query.QueryType,
      queryText: query.QueryText === undefined ? "" : query.QueryText,
      querySelection: JSON.stringify(query.QuerySelection)
    })
  }

  async HideApp(): Promise<void> {
    await this.invokeMethod("HideApp", {})
  }

  async Log(msg: string): Promise<void> {
    await this.invokeMethod("Log", { msg })
  }

  async ShowApp(): Promise<void> {
    await this.invokeMethod("ShowApp", {})
  }

  async Notify(title: string, description: string | undefined): Promise<void> {
    await this.invokeMethod("Notify", {
      title,
      description: description === undefined ? "" : description
    })
  }

  async GetTranslation(key: string): Promise<string> {
    return (await this.invokeMethod("GetTranslation", { key })) as string
  }

  async GetSetting(key: string): Promise<string> {
    return (await this.invokeMethod("GetSetting", { key })) as string
  }

  async SaveSetting(key: string, value: string, isPlatformSpecific: boolean): Promise<void> {
    await this.invokeMethod("SaveSetting", { key, value, isPlatformSpecific: isPlatformSpecific.toString() })
  }

  async OnSettingChanged(callback: (key: string, value: string) => void): Promise<void> {
    const callbackId = crypto.randomUUID()
    this.settingChangeCallbacks.set(callbackId, callback)
    await this.invokeMethod("OnPluginSettingChanged", { callbackId })
  }
}
