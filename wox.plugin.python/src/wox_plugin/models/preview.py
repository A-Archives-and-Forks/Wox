"""
Wox Preview Models

This module provides preview models for displaying rich content in Wox results.

Previews allow plugins to show detailed information, images, files, and web
content in a dedicated preview panel when a result is selected.
"""

from dataclasses import dataclass, field
from enum import Enum
import json
from typing import Any, Dict, List


class WoxPreviewType(str, Enum):
    """
    Enumeration of supported preview types in Wox.

    Each type represents a different way to render preview content:
    - MARKDOWN: Render Markdown formatted text
    - TEXT: Plain text display
    - IMAGE: Display an image (using WoxImage)
    - URL: Load and display a web page
    - FILE: Display a file (various formats supported)
    - FILE_LIST: Display multiple file paths using WoxPreviewFileListData JSON
    - REMOTE: Load preview data from a remote URL
    """

    MARKDOWN = "markdown"
    """
    Markdown formatted text.

    The preview_data should contain Markdown markup which will be
    rendered to HTML for display. Supports standard Markdown syntax
    including headers, lists, code blocks, links, etc.

    Example:
        preview = WoxPreview(
            preview_type=WoxPreviewType.MARKDOWN,
            preview_data="# Header\\n\\n- Item 1\\n- Item 2"
        )
    """

    TEXT = "text"
    """
    Plain text display.

    The preview_data is displayed as-is without any formatting.
    Newlines and whitespace are preserved. Use this for simple
    text content or when you don't need rich formatting.

    Example:
        preview = WoxPreview(
            preview_type=WoxPreviewType.TEXT,
            preview_data="This is plain text.\\n\\nLine 2\\nLine 3"
        )
    """

    IMAGE = "image"
    """
    Display an image.

    The preview_data should be a WoxImage serialized to string format
    (i.e., the result of calling str(WoxImage(...))).

    Example:
        icon = WoxImage.new_absolute("/path/to/image.png")
        preview = WoxPreview(
            preview_type=WoxPreviewType.IMAGE,
            preview_data=str(icon)  # "absolute:/path/to/image.png"
        )
    """

    URL = "url"
    """
    Load and display a web page.

    The preview_data should be a URL to a web page. Wox will load
    and render the page in an embedded browser.

    Note: This may have security and privacy implications as the
    page can execute JavaScript and access cookies.

    Example:
        preview = WoxPreview(
            preview_type=WoxPreviewType.URL,
            preview_data="https://example.com"
        )
    """

    FILE = "file"
    """
    Display a file from the file system.

    The preview_data should be a file path. Wox will attempt to
    render the file based on its extension. Supported formats include:
    - Markdown files (.md)
    - Image files (.jpg, .png, .gif, .svg, etc.)
    - PDF files (.pdf)
    - Text files (.txt, .json, .xml, etc.)

    Example:
        preview = WoxPreview(
            preview_type=WoxPreviewType.FILE,
            preview_data="/path/to/document.pdf"
        )
    """

    FILE_LIST = "file_list"
    """
    Display a structured list of files.

    The preview_data should be WoxPreviewFileListData.to_json(). This public
    data object keeps SDK plugins aligned with the core file-list renderer
    instead of requiring each plugin to hand-write the JSON field names.

    Example:
        data = WoxPreviewFileListData(file_paths=["/path/to/a.txt", "/path/to/b.txt"])
        preview = WoxPreview(
            preview_type=WoxPreviewType.FILE_LIST,
            preview_data=data.to_json()
        )
    """

    REMOTE = "remote"
    """
    Load preview data from a remote URL.

    The preview_data should be a URL that returns WoxPreview JSON data
    when fetched. This allows plugins to dynamically generate previews
    from an external service.

    Example:
        preview = WoxPreview(
            preview_type=WoxPreviewType.REMOTE,
            preview_data="https://api.example.com/preview/123"
        )
    """


@dataclass
class WoxPreviewFileListData:
    """
    Structured data for WoxPreviewType.FILE_LIST.

    The JSON field is intentionally `filePaths` because the core and Flutter
    renderer already use that lower-camel contract. A named SDK model prevents
    plugins from drifting into alternate payload shapes such as raw arrays.
    """

    file_paths: List[str] = field(default_factory=list)
    """
    Absolute or plugin-resolved file paths to render in the file-list preview.
    """

    def to_json(self) -> str:
        """
        Convert to the JSON payload expected by WoxPreview.preview_data.
        """
        return json.dumps({"filePaths": self.file_paths})

    @classmethod
    def from_json(cls, json_data: Dict[str, Any]) -> "WoxPreviewFileListData":
        """
        Create file-list preview data from a decoded JSON object.
        """
        raw_paths = json_data.get("filePaths", [])
        return cls(file_paths=[str(item) for item in raw_paths] if isinstance(raw_paths, list) else [])

    @classmethod
    def from_preview_data(cls, preview_data: str) -> "WoxPreviewFileListData":
        """
        Decode the string stored in WoxPreview.preview_data.
        """
        decoded = json.loads(preview_data)
        return cls.from_json(decoded if isinstance(decoded, dict) else {})


class WoxPreviewScrollPosition(str, Enum):
    """
    Enumeration of preview scroll positions.

    Controls where the preview content is scrolled when first displayed.
    """

    BOTTOM = "bottom"
    """
    Scroll to the bottom after preview first shows.

    Use this for content that grows from the top (like logs, chat messages,
    or terminal output) so the user sees the most recent content first.
    """


@dataclass
class WoxPreview:
    """
    Preview model for displaying rich content in Wox results.

    Previews are shown in a side panel when a result is selected, allowing
    plugins to display detailed information without cluttering the main
    results list.

    Attributes:
        preview_type: The type of preview content to display
        preview_data: The actual content data (format depends on preview_type)
        preview_properties: Optional properties for preview customization
        scroll_position: Initial scroll position when preview is shown

    Example usage:
        # Markdown preview
        preview = WoxPreview(
            preview_type=WoxPreviewType.MARKDOWN,
            preview_data="# Documentation\\n\\nThis is **bold** text."
        )

        # Image preview
        icon = WoxImage.new_absolute("/path/to/screenshot.png")
        preview = WoxPreview(
            preview_type=WoxPreviewType.IMAGE,
            preview_data=str(icon)
        )

        # File preview
        preview = WoxPreview(
            preview_type=WoxPreviewType.FILE,
            preview_data="/path/to/readme.md"
        )
    """

    preview_type: WoxPreviewType = field(default=WoxPreviewType.TEXT)
    """
    The type of preview content to display.

    Determines how the preview_data is interpreted and rendered.
    """

    preview_data: str = field(default="")
    """
    The actual preview content.

    The format of this field depends on preview_type:
    - MARKDOWN: Markdown markup string
    - TEXT: Plain text string
    - IMAGE: WoxImage serialized as "type:value" string
    - URL: HTTP/HTTPS URL
    - FILE: File system path
    - FILE_LIST: WoxPreviewFileListData JSON string
    - REMOTE: URL that returns WoxPreview JSON
    """

    preview_properties: Dict[str, str] = field(default_factory=dict)
    """
    Optional properties for preview customization.

    This dictionary can contain additional properties that modify
    how the preview is displayed. The available properties depend
    on the preview_type and Wox version.

    Common properties may include:
    - "height": Maximum height for the preview
    - "theme": Theme override for code blocks
    - Custom plugin-specific properties
    """

    scroll_position: WoxPreviewScrollPosition = field(default=WoxPreviewScrollPosition.BOTTOM)
    """
    Initial scroll position when preview is first displayed.

    Controls where the content is scrolled when the preview appears.
    Default is BOTTOM which scrolls to the end of the content.
    """

    def to_json(self) -> str:
        """
        Convert to JSON string with camelCase naming.

        The output uses camelCase property names for compatibility
        with the Wox C# backend.

        Returns:
            JSON string representation of this preview
        """
        return json.dumps(
            {
                "PreviewType": self.preview_type,
                "PreviewData": self.preview_data,
                "PreviewProperties": self.preview_properties,
                "ScrollPosition": self.scroll_position,
            }
        )

    @classmethod
    def from_json(cls, json_str: str) -> "WoxPreview":
        """
        Create from JSON string with camelCase naming.

        Args:
            json_str: JSON string containing preview data

        Returns:
            A new WoxPreview instance
        """
        data = json.loads(json_str)

        if not data.get("PreviewType"):
            data["PreviewType"] = WoxPreviewType.TEXT

        if not data.get("ScrollPosition"):
            data["ScrollPosition"] = WoxPreviewScrollPosition.BOTTOM

        return cls(
            preview_type=WoxPreviewType(data.get("PreviewType")),
            preview_data=data.get("PreviewData", ""),
            preview_properties=data.get("PreviewProperties", {}),
            scroll_position=WoxPreviewScrollPosition(data.get("ScrollPosition")),
        )
