import "winston-daily-rotate-file"
import { WebSocketServer } from "ws"
import { handleRequestFromWox, PluginJsonRpcRequest, PluginJsonRpcResponse, PluginJsonRpcTypeRequest, PluginJsonRpcTypeResponse } from "./jsonrpc"
import { logger } from "./logger"
import * as crypto from "crypto"
import Deferred from "promise-deferred"
import { NewTraceContext, TraceIdKey } from "./trace"
import { NewContextWithValue } from "@wox-launcher/wox-plugin"

if (process.argv.length < 5) {
  console.error("Usage: node node.js <port> <logDirectory> <woxPid>")
  process.exit(1)
}

const port = process.argv[2]
const woxPid = process.argv[4]
const hostId = `node-${crypto.randomUUID()}`

const startupContext = NewTraceContext()
logger.info(startupContext, "----------------------------------------")
logger.info(startupContext, `start nodejs host: ${hostId}`)
logger.info(startupContext, `port: ${port}`)
logger.info(startupContext, `wox pid: ${woxPid}`)

//check wox process is alive, otherwise exit
setInterval(() => {
  try {
    process.kill(Number.parseInt(woxPid), 0)
  } catch (e) {
    logger.error(startupContext, `wox process is not alive, exit`)
    process.exit(1)
  }
}, 1000)

export const waitingForResponse: {
  [key: string]: Deferred.Deferred<unknown>
} = {}

const wss = new WebSocketServer({ port: Number.parseInt(port) })
wss.on("connection", function connection(ws) {
  logger.updateWebSocket(ws)

  ws.on("error", function(error) {
    logger.updateWebSocket(undefined)
    logger.error(NewTraceContext(), `[${hostId}] connection error: ${error.message}`)
  })

  ws.on("close", function close(code, reason) {
    logger.updateWebSocket(undefined)
    logger.info(NewTraceContext(), `[${hostId}] connection closed, code: ${code}, reason: ${reason}`)
  })

  ws.on("ping", function ping() {
    ws.pong()
  })

  ws.on("message", function message(data) {
    try {
      const msg = `${data}`
      // logger.debug(crypto.randomUUID(), `receive message: ${msg}`)

      if (msg.indexOf(PluginJsonRpcTypeResponse) >= 0) {
        handleResponseFromWox(msg)
      } else if (msg.indexOf(PluginJsonRpcTypeRequest) >= 0) {
        handleRequest(msg)
      } else {
        logger.error(NewTraceContext(), `unknown message type: ${msg}`)
      }
    } catch (e) {
      logger.error(NewTraceContext(), `receive and handle msg error: ${data}, err: ${e}`)
    }
  })

  function handleRequest(msg: string) {
    let jsonRpcRequest: PluginJsonRpcRequest
    try {
      jsonRpcRequest = JSON.parse(msg) as PluginJsonRpcRequest
    } catch (e) {
      logger.error(NewTraceContext(), `error parsing json: ${e}, data: ${msg}`)
      return
    }

    if (jsonRpcRequest === undefined) {
      logger.error(NewTraceContext(), `jsonRpcRequest is undefined`)
      return
    }

    const ctx = NewContextWithValue(TraceIdKey, jsonRpcRequest.TraceId)

    logger.debug(ctx, `receive request from wox: ${JSON.stringify(jsonRpcRequest)}`)

    // eslint-disable-next-line @typescript-eslint/ban-ts-comment
    // @ts-ignore
    handleRequestFromWox(ctx, jsonRpcRequest, ws)
      .then((result: unknown) => {
        const response: PluginJsonRpcResponse = {
          TraceId: jsonRpcRequest.TraceId,
          Id: jsonRpcRequest.Id,
          Method: jsonRpcRequest.Method,
          Type: PluginJsonRpcTypeResponse,
          Result: result
        }
        //logger.info(`[${jsonRpcRequest.PluginName}] handle request successfully: ${JSON.stringify(response)}, ${ws.readyState}`)
        ws.send(JSON.stringify(response), (error?: Error) => {
          if (error) {
            logger.error(ctx, `[${jsonRpcRequest.PluginName}] send response failed: ${error.message}`)
          }
        })
      })
      .catch((error: Error) => {
        const response: PluginJsonRpcResponse = {
          TraceId: jsonRpcRequest.TraceId,
          Id: jsonRpcRequest.Id,
          Method: jsonRpcRequest.Method,
          Type: PluginJsonRpcTypeResponse,
          Error: error.message
        }
        logger.error(ctx, `[${jsonRpcRequest.PluginName}] handle request failed: ${error.message}, stack: ${error.stack}`)
        ws.send(JSON.stringify(response), (error?: Error) => {
          if (error) {
            logger.error(ctx, `[${jsonRpcRequest.PluginName}] send response failed: ${error.message}, stack: ${error.stack}`)
          }
        })
      })
  }

  function handleResponseFromWox(msg: string) {
    let pluginJsonRpcResponse: PluginJsonRpcResponse
    try {
      pluginJsonRpcResponse = JSON.parse(msg) as PluginJsonRpcResponse
    } catch (e) {
      logger.error(NewTraceContext(), `error parsing response json: ${e}, data: ${msg}`)
      return
    }

    if (pluginJsonRpcResponse === undefined) {
      logger.error(NewTraceContext(), `pluginJsonRpcResponse is undefined`)
      return
    }

    if (pluginJsonRpcResponse.Id === undefined) {
      logger.error(NewTraceContext(), `pluginJsonRpcResponse.Id is undefined`)
      return
    }

    const ctx = NewContextWithValue(TraceIdKey, pluginJsonRpcResponse.TraceId)

    const promiseInstance = waitingForResponse[pluginJsonRpcResponse.Id]
    if (promiseInstance === undefined) {
      logger.error(ctx, `waitingForResponse[${pluginJsonRpcResponse.Id}] is undefined`)
      return
    }

    promiseInstance.resolve(pluginJsonRpcResponse.Result)
  }
})
