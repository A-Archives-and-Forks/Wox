import styled from "styled-components"
import WoxQueryBox, { WoxQueryBoxRefHandler } from "../components/query/WoxQueryBox.tsx"
import React, { useEffect, useReducer, useRef } from "react"
import WoxQueryResult, { WoxQueryResultRefHandler } from "../components/query/WoxQueryResult.tsx"
import { WOXMESSAGE } from "../entity/WoxMessage.typings"
import { WoxMessageHelper } from "../utils/WoxMessageHelper.ts"
import { WoxMessageRequestMethod, WoxMessageRequestMethodEnum } from "../enums/WoxMessageRequestMethodEnum.ts"
import { useInterval } from "usehooks-ts"
import { WoxMessageMethodEnum } from "../enums/WoxMessageMethodEnum.ts"
import { WoxLogHelper } from "../utils/WoxLogHelper.ts"
import { WoxUIHelper } from "../utils/WoxUIHelper.ts"
import Mousetrap from "mousetrap"
import { WoxThemeHelper } from "../utils/WoxThemeHelper.ts"
import { Theme } from "../entity/Theme.typings"
import { WoxPositionTypeEnum } from "../enums/WoxPositionTypeEnum.ts"
import { WoxLastQueryMode, WoxLastQueryModeEnum } from "../enums/WoxLastQueryModeEnum.ts"

export default () => {
  const [_, forceUpdate] = useReducer(x => x + 1, 0)
  const woxQueryBoxRef = React.useRef<WoxQueryBoxRefHandler>(null)
  const woxQueryResultRef = React.useRef<WoxQueryResultRefHandler>(null)
  const requestTimeoutId = useRef<number>(0)
  const refreshTotalCount = useRef<number>(0)
  const hasLatestQueryResult = useRef<boolean>(true)
  const currentQueryId = useRef<string>()
  const latestChangedQuery = useRef<WOXMESSAGE.ChangedQuery>({} as WOXMESSAGE.ChangedQuery)
  const latestQueryHistories = useRef<WOXMESSAGE.QueryHistory[]>([])
  const lastQueryMode = useRef<WoxLastQueryMode>(WoxLastQueryModeEnum.WoxLastQueryModeEmpty.code)
  const selectedQueryHistoryIndex = useRef<number>(0)
  const fullResultList = useRef<WOXMESSAGE.WoxMessageResponseResult[]>([])

  /**
   * Deal with user input change
   * @param query
   */
  const onQueryChange = (query: WOXMESSAGE.ChangedQuery) => {
    latestChangedQuery.current = query
    woxQueryResultRef.current?.hideActionList()
    currentQueryId.current = crypto.randomUUID()
    fullResultList.current = []
    clearTimeout(requestTimeoutId.current)
    hasLatestQueryResult.current = false
    WoxMessageHelper.getInstance().sendQueryMessage(
      {
        queryId: currentQueryId.current,
        queryType: query.QueryType,
        queryText: query.QueryText || "",
        querySelection: JSON.stringify(query.QuerySelection || {})
      },
      handleQueryCallback
    )
    // @ts-ignore
    requestTimeoutId.current = setTimeout(() => {
      if (!hasLatestQueryResult.current) {
        woxQueryResultRef.current?.clearResultList()
      }
    }, 200)
  }

  const refreshResults = async () => {
    let needUpdate = false
    let preview = false
    const currentCount = refreshTotalCount.current
    for (const [i, result] of fullResultList.current.entries()) {
      if (result.RefreshInterval > 0) {
        if (currentCount % result.RefreshInterval === 0) {
          const refreshableResult = {
            Title: result.Title,
            SubTitle: result.SubTitle,
            Icon: result.Icon,
            Preview: result.Preview,
            ContextData: result.ContextData,
            RefreshInterval: result.RefreshInterval
          } as WOXMESSAGE.WoxRefreshableResult

          let response = await WoxMessageHelper.getInstance().sendMessage(WoxMessageMethodEnum.REFRESH.code, {
            resultId: result.Id,
            refreshableResult: JSON.stringify(refreshableResult)
          })
          if (response.Success) {
            const newResult = response.Data as WOXMESSAGE.WoxRefreshableResult
            if (newResult) {
              fullResultList.current[i].Title = newResult.Title
              fullResultList.current[i].SubTitle = newResult.SubTitle
              fullResultList.current[i].Icon = newResult.Icon
              fullResultList.current[i].Preview = newResult.Preview
              fullResultList.current[i].ContextData = newResult.ContextData
              fullResultList.current[i].RefreshInterval = newResult.RefreshInterval
              preview = !!newResult.Preview.PreviewType
              needUpdate = true
            }
          } else {
            WoxLogHelper.getInstance().log(`refresh [${result.Title}] failed: ${response.Data}`)
          }
        }
      }
    }

    if (needUpdate) {
      woxQueryResultRef.current?.changeResultList(preview, [...fullResultList.current])
    }
  }

  /*
    Because the query callback will be called multiple times, so we need to filter the result by query text
   */
  const handleQueryCallback = (results: WOXMESSAGE.WoxMessageResponseResult[]) => {
    console.log(results)
    fullResultList.current = fullResultList.current.concat(results).filter(result => {
      if (result.QueryId === currentQueryId.current) {
        hasLatestQueryResult.current = true
      }
      return result.QueryId === currentQueryId.current
    })

    //sort fullResultList order by score desc
    fullResultList.current.sort((a, b) => {
      return b.Score - a.Score
    })

    let preview = false
    fullResultList.current = fullResultList.current.map((result, index) => {
      preview = !!result.Preview.PreviewType
      return Object.assign({ ...result, Index: index })
    })

    woxQueryResultRef.current?.changeResultList(preview, [...fullResultList.current])
  }

  /*
    Deal with global request event
   */
  const handleRequestCallback = async (message: WOXMESSAGE.WoxMessage) => {
    if (message.Method === WoxMessageRequestMethodEnum.ChangeQuery.code) {
      await changeQuery(message.Data as WOXMESSAGE.ChangedQuery)
    }
    if (message.Method === WoxMessageRequestMethodEnum.HideApp.code) {
      await hideWoxWindow()
    }
    if (message.Method === WoxMessageRequestMethodEnum.ShowApp.code) {
      await showWoxWindow(message.Data as WOXMESSAGE.ShowAppParams)
    }
    if (message.Method === WoxMessageRequestMethodEnum.ToggleApp.code) {
      await toggleWoxWindow(message.Data as WOXMESSAGE.ShowAppParams)
    }
    if (message.Method === WoxMessageRequestMethodEnum.ChangeTheme.code) {
      await changeTheme(message.Data as string)
    }
    if (message.Method === WoxMessageRequestMethodEnum.OpenSettingDialog.code) {
      await WoxUIHelper.getInstance().openSettingWindow()
    }
    if (message.Method === WoxMessageRequestMethodEnum.OpenDevTools.code) {
      await WoxUIHelper.getInstance().openDevTools()
    }
  }

  /*
    Hide wox window
   */
  const hideWoxWindow = async () => {
    const isVisible = await WoxUIHelper.getInstance().isVisible()
    if (!isVisible) {
      //already hide
      return
    }

    if (lastQueryMode.current === WoxLastQueryModeEnum.WoxLastQueryModePreserve.code) {
      //skip the first one, because it's the current query
      selectedQueryHistoryIndex.current = 0
    } else {
      selectedQueryHistoryIndex.current = -1
    }

    await WoxUIHelper.getInstance().hideWindow()
    await WoxMessageHelper.getInstance().sendMessage(WoxMessageMethodEnum.VISIBILITY_CHANGED.code, {
      isVisible: "false",
      query: JSON.stringify(latestChangedQuery.current || {})
    })
  }

  /*
    Show wox window
  */
  const showWoxWindow = async (param: WOXMESSAGE.ShowAppParams) => {
    latestQueryHistories.current = param.QueryHistories || []
    lastQueryMode.current = param.LastQueryMode || WoxLastQueryModeEnum.WoxLastQueryModeEmpty.code
    if (param.Position.Type === WoxPositionTypeEnum.WoxPositionTypeMouseScreen.code) {
      await WoxUIHelper.getInstance().setPosition(param.Position.X, param.Position.Y)
    }
    if (param.SelectAll) {
      woxQueryBoxRef.current?.selectAll()
    }
    await WoxUIHelper.getInstance().showWindow()
    woxQueryBoxRef.current?.focus()
    // on windows, the window size will randomly change, which is weird, so we need to force resize window for temp solution
    woxQueryResultRef.current?.forceResizeWindow()
    await WoxMessageHelper.getInstance().sendMessage(WoxMessageMethodEnum.VISIBILITY_CHANGED.code, {
      isVisible: "true",
      query: woxQueryBoxRef.current?.getQuery() || ""
    })
  }

  /*
    Toggle wox window
  */
  const toggleWoxWindow = async (param: WOXMESSAGE.ShowAppParams) => {
    const isVisible = await WoxUIHelper.getInstance().isVisible()
    if (isVisible) {
      await hideWoxWindow()
    } else {
      await showWoxWindow(param)
    }
  }

  /*
    Change query text
  */
  const changeQuery = async (query: WOXMESSAGE.ChangedQuery) => {
    woxQueryBoxRef.current?.changeQuery(query)
  }

  const changeTheme = async (theme: string) => {
    WoxThemeHelper.getInstance()
      .changeTheme(JSON.parse(theme) as Theme)
      .then(_ => {
        WoxUIHelper.getInstance().setBackgroundColor(WoxThemeHelper.getInstance().getTheme().AppBackgroundColor)
        forceUpdate()
        woxQueryResultRef.current?.forceResizeWindow()
      })
  }

  const bindKeyboardEvent = () => {
    Mousetrap.bind("esc", event => {
      if (woxQueryResultRef.current?.isActionListShown()) {
        woxQueryResultRef.current?.hideActionList()
        woxQueryBoxRef.current?.focus()
      } else {
        woxQueryResultRef.current?.resetMouseIndex()
        hideWoxWindow()
      }
      event.preventDefault()
      event.stopPropagation()
    })
    Mousetrap.bind("down", event => {
      woxQueryResultRef.current?.moveDown()
      event.preventDefault()
      event.stopPropagation()
    })
    Mousetrap.bind("up", event => {
      woxQueryResultRef.current?.moveUp()
      event.preventDefault()
      event.stopPropagation()
    })
    Mousetrap.bind("ctrl+up", event => {
      if (selectedQueryHistoryIndex.current < latestQueryHistories.current.length - 1) {
        selectedQueryHistoryIndex.current = selectedQueryHistoryIndex.current + 1
        changeQuery(latestQueryHistories.current[selectedQueryHistoryIndex.current].Query).then(_ => {
          woxQueryBoxRef.current?.selectAll()
        })
      }
      event.preventDefault()
      event.stopPropagation()
    })
    Mousetrap.bind("ctrl+down", event => {
      if (selectedQueryHistoryIndex.current > 0) {
        selectedQueryHistoryIndex.current = selectedQueryHistoryIndex.current - 1
        changeQuery(latestQueryHistories.current[selectedQueryHistoryIndex.current].Query).then(_ => {
          woxQueryBoxRef.current?.selectAll()
        })
      }
      event.preventDefault()
      event.stopPropagation()
    })
    Mousetrap.bind("enter", event => {
      woxQueryResultRef.current?.doAction()
      event.preventDefault()
      event.stopPropagation()
    })
    Mousetrap.bind("command+j", event => {
      woxQueryResultRef.current?.toggleActionList().then(actionListVisibility => {
        if (!actionListVisibility) {
          woxQueryBoxRef.current?.focus()
        }
      })
      event.preventDefault()
      event.stopPropagation()
    })
  }

  useInterval(async () => {
    refreshTotalCount.current = refreshTotalCount.current + 100
    await refreshResults()
  }, 100)

  useEffect(() => {
    WoxMessageHelper.getInstance().initialRequestCallback(handleRequestCallback)
    bindKeyboardEvent()
  }, [])

  return (
    <Style theme={WoxThemeHelper.getInstance().getTheme()} className={"wox-launcher"}>
      <WoxQueryBox
        ref={woxQueryBoxRef}
        onQueryChange={onQueryChange}
        onClick={() => {
          woxQueryResultRef.current?.hideActionList()
        }}
      />

      <WoxQueryResult
        ref={woxQueryResultRef}
        callback={async (method: WoxMessageRequestMethod) => {
          if (method === WoxMessageRequestMethodEnum.HideApp.code) {
            await hideWoxWindow()
          }
        }}
      />
    </Style>
  )
}

const Style = styled.div<{ theme: Theme }>`
  background-color: ${props => props.theme.AppBackgroundColor};
  padding-top: ${props => props.theme.AppPaddingTop}px;
  padding-right: ${props => props.theme.AppPaddingRight}px;
  padding-bottom: ${props => props.theme.AppPaddingBottom}px;
  padding-left: ${props => props.theme.AppPaddingLeft}px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
`
