import React, { useEffect, useState } from "react"
import { Box, ListItemIcon, ListItemText, MenuItem, MenuList, Paper } from "@mui/material"
import styled from "styled-components"
import WoxSettingGeneral, { WoxSettingGeneralRefHandler } from "../components/settings/WoxSettingGeneral.tsx"
import SettingsOutlinedIcon from "@mui/icons-material/SettingsOutlined"
import ExtensionIcon from "@mui/icons-material/Extension"
import DarkModeIcon from "@mui/icons-material/DarkMode"
import { WoxSettingHelper } from "../utils/WoxSettingHelper.ts"
import WoxSettingPlugins from "../components/settings/WoxSettingPlugins.tsx"
import WoxSettingThemes from "../components/settings/WoxSettingThemes.tsx"

export default () => {
  const menuList = [
    {
      Icon: SettingsOutlinedIcon,
      Text: "General"
    },
    {
      Icon: ExtensionIcon,
      Text: "Plugins"
    },
    {
      Icon: DarkModeIcon,
      Text: "Themes"
    }
  ]
  const [selectedIndex, setSelectedIndex] = useState(0)
  const woxSettingGeneralRef = React.useRef<WoxSettingGeneralRefHandler>(null)

  const handleMenuItemClick = (event: React.MouseEvent<HTMLLIElement, MouseEvent>, index: number) => {
    setSelectedIndex(index)
    event.preventDefault()
    event.stopPropagation()
  }

  useEffect(() => {
    WoxSettingHelper.getInstance()
      .loadSetting()
      .then(_ => {
        woxSettingGeneralRef.current?.initialize()
      })
  }, [])

  return (
    <Style>
      <Box sx={{ flexGrow: 1, display: "flex", height: "100%" }}>
        <Paper className={"menu-container"} sx={{ width: "260px", background: "rgb(35 39 45 / 50%)", height: "100%", borderRadius: 0 }}>
          <div className={"drag-area"}>&nbsp;</div>
          <MenuList sx={{ paddingLeft: "16px", paddingRight: "16px" }}>
            {menuList.map((item, index) => {
              return (
                <MenuItem
                  sx={{ color: "white", margin: "0px 0px 4px", boxSizing: "content-box" }}
                  key={index}
                  selected={selectedIndex === index}
                  onClick={event => {
                    handleMenuItemClick(event, index)
                  }}
                >
                  <ListItemIcon sx={{ color: "white" }}>
                    <item.Icon fontSize="small" />
                  </ListItemIcon>
                  <ListItemText>{item.Text}</ListItemText>
                </MenuItem>
              )
            })}
          </MenuList>
        </Paper>

        <div className={"setting-container"}>
          <div className={"drag-area"}>&nbsp;</div>
          <div className={"setting-item"} style={{ display: selectedIndex === 0 ? "block" : "none" }}>
            <WoxSettingGeneral ref={woxSettingGeneralRef} />
          </div>
          <div className={"setting-item"} style={{ display: selectedIndex === 1 ? "block" : "none" }}>
            <WoxSettingPlugins />
          </div>
          <div className={"setting-item"} style={{ display: selectedIndex === 2 ? "block" : "none" }}>
            <WoxSettingThemes />
          </div>
        </div>
      </Box>
    </Style>
  )
}

const Style = styled.div`
  width: 100%;
  height: 100%;

  .drag-area {
    -webkit-app-region: drag;
    background-color: transparent;
    height: 32px;
    width: 100%;
  }

  .menu-container {
    .Mui-selected {
      border: 1px solid #4480f8 !important;
      border-radius: 5px;
    }
  }

  .setting-container {
    width: 100%;
    height: 100%;
    background-color: #313335;
  }
`
