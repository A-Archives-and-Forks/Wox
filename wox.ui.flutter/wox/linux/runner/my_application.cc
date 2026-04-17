#include "my_application.h"

#include <cairo.h>
#include <flutter_linux/flutter_linux.h>
#include <gdk/gdk.h>
#include <math.h>
#include <string>
#include <stdarg.h>
#include <vector>
#ifdef GDK_WINDOWING_X11
#include <X11/Xatom.h>
#include <X11/keysym.h>
#include <X11/Xlib.h>
#include <X11/extensions/XTest.h>
#include <gdk/gdkx.h>
#endif

#include "flutter/generated_plugin_registrant.h"

struct _MyApplication
{
  GtkApplication parent_instance;
  char **dart_entrypoint_arguments;
  GtkWindow *window; // Store reference to the main window
  gulong previous_active_window;
  gboolean restore_previous_window_on_hide;
};

// Global variable to store method channel for window events
static FlMethodChannel *g_method_channel = nullptr;

G_DEFINE_TYPE(MyApplication, my_application, GTK_TYPE_APPLICATION)

static void log(const char *format, ...)
{
  // va_list args;
  // va_start(args, format);
  // g_logv(G_LOG_DOMAIN, G_LOG_LEVEL_MESSAGE, format, args);
  // va_end(args);
}

// Function to draw rounded rectangle
static void cairo_rounded_rectangle(cairo_t *cr, double x, double y,
                                    double width, double height,
                                    double radius)
{
  cairo_new_sub_path(
      cr); // Fix function name: cairo_new_subpath -> cairo_new_sub_path
  cairo_arc(cr, x + radius, y + radius, radius, M_PI, 3 * M_PI / 2);
  cairo_line_to(cr, x + width - radius, y);
  cairo_arc(cr, x + width - radius, y + radius, radius, 3 * M_PI / 2, 0);
  cairo_line_to(cr, x + width, y + height - radius);
  cairo_arc(cr, x + width - radius, y + height - radius, radius, 0, M_PI / 2);
  cairo_line_to(cr, x + radius, y + height);
  cairo_arc(cr, x + radius, y + height - radius, radius, M_PI / 2, M_PI);
  cairo_close_path(cr);
}

static void set_window_shape(GtkWindow *window)
{
  GdkWindow *gdk_window = gtk_widget_get_window(GTK_WIDGET(window));
  if (!gdk_window)
  {
    return;
  }

  int width, height;
  gtk_window_get_size(window, &width, &height);

  cairo_surface_t *surface =
      cairo_image_surface_create(CAIRO_FORMAT_A1, width, height);
  cairo_t *cr = cairo_create(surface);

  cairo_set_source_rgba(cr, 1, 1, 1, 1); // white fill
  cairo_set_operator(cr, CAIRO_OPERATOR_SOURCE);
  cairo_rounded_rectangle(cr, 0, 0, width, height, 10); // rounded radius is 10
  cairo_fill(cr);

  cairo_destroy(cr);

  cairo_region_t *region = gdk_cairo_region_create_from_surface(surface);
  gdk_window_shape_combine_region(gdk_window, region, 0, 0);
  cairo_region_destroy(region);

  cairo_surface_destroy(surface);
}

// Callback function to handle window size changes
static void on_size_allocate(GtkWidget *widget, GdkRectangle *allocation,
                             gpointer user_data)
{
  set_window_shape(GTK_WINDOW(user_data));
}

// Callback function to handle window focus-out event
static gboolean on_window_focus_out(GtkWidget *widget, GdkEventFocus *event,
                                    gpointer user_data)
{
  MyApplication *self = MY_APPLICATION(user_data);
  log("FLUTTER: Window lost focus");

  if (self != nullptr && gtk_widget_get_visible(widget))
  {
    self->restore_previous_window_on_hide = FALSE;
    self->previous_active_window = 0;
  }

  // Notify Flutter through method channel
  if (g_method_channel != nullptr)
  {
    g_autoptr(FlValue) args = fl_value_new_null();
    fl_method_channel_invoke_method(g_method_channel, "onWindowBlur", args,
                                    nullptr, nullptr, nullptr);
  }

  // Return FALSE to allow the event to propagate further
  return FALSE;
}

#ifdef GDK_WINDOWING_X11
static Display *get_x11_display(GtkWindow *window)
{
  GdkWindow *gdk_window = gtk_widget_get_window(GTK_WIDGET(window));
  if (gdk_window == nullptr || !GDK_IS_X11_WINDOW(gdk_window))
  {
    return nullptr;
  }

  return GDK_DISPLAY_XDISPLAY(gdk_window_get_display(gdk_window));
}

static Window get_x11_window_id(GtkWindow *window)
{
  GdkWindow *gdk_window = gtk_widget_get_window(GTK_WIDGET(window));
  if (gdk_window == nullptr || !GDK_IS_X11_WINDOW(gdk_window))
  {
    return None;
  }

  return GDK_WINDOW_XID(gdk_window);
}

static Window get_active_x11_window(GtkWindow *window)
{
  Display *display = get_x11_display(window);
  if (display == nullptr)
  {
    return None;
  }

  Atom net_active_window = XInternAtom(display, "_NET_ACTIVE_WINDOW", True);
  if (net_active_window == None)
  {
    return None;
  }

  Atom actual_type = None;
  int actual_format = 0;
  unsigned long item_count = 0;
  unsigned long bytes_after = 0;
  unsigned char *data = nullptr;
  Window active_window = None;

  int status = XGetWindowProperty(display, DefaultRootWindow(display),
                                  net_active_window, 0, 1, False, XA_WINDOW,
                                  &actual_type, &actual_format, &item_count,
                                  &bytes_after, &data);
  if (status == Success && actual_type == XA_WINDOW && actual_format == 32 &&
      item_count == 1 && data != nullptr)
  {
    active_window = *(reinterpret_cast<Window *>(data));
  }

  if (data != nullptr)
  {
    XFree(data);
  }

  return active_window;
}

static void save_previous_active_window(MyApplication *self)
{
  if (self == nullptr || self->window == nullptr)
  {
    return;
  }

  Window current_window = get_x11_window_id(self->window);
  Window active_window = get_active_x11_window(self->window);
  if (active_window == None || active_window == current_window)
  {
    return;
  }

  self->previous_active_window = active_window;
  self->restore_previous_window_on_hide = TRUE;
}

static void restore_previous_active_window(MyApplication *self)
{
  if (self == nullptr || self->window == nullptr)
  {
    return;
  }

  Window previous_window = static_cast<Window>(self->previous_active_window);
  self->previous_active_window = 0;
  if (previous_window == None)
  {
    return;
  }

  Display *display = get_x11_display(self->window);
  Window current_window = get_x11_window_id(self->window);
  if (display == nullptr || previous_window == current_window)
  {
    return;
  }

  XWindowAttributes attributes;
  if (XGetWindowAttributes(display, previous_window, &attributes) == 0 ||
      attributes.map_state != IsViewable)
  {
    return;
  }

  Atom net_active_window = XInternAtom(display, "_NET_ACTIVE_WINDOW", False);
  if (net_active_window == None)
  {
    return;
  }

  XEvent event;
  memset(&event, 0, sizeof(event));
  event.xclient.type = ClientMessage;
  event.xclient.window = previous_window;
  event.xclient.message_type = net_active_window;
  event.xclient.format = 32;
  event.xclient.data.l[0] = 2;
  event.xclient.data.l[1] = CurrentTime;

  XSendEvent(display, DefaultRootWindow(display), False,
             SubstructureRedirectMask | SubstructureNotifyMask, &event);
  XFlush(display);
}
#else
static void save_previous_active_window(MyApplication *self)
{
  if (self != nullptr)
  {
    self->previous_active_window = 0;
    self->restore_previous_window_on_hide = FALSE;
  }
}

static void restore_previous_active_window(MyApplication *self)
{
  if (self != nullptr)
  {
    self->previous_active_window = 0;
    self->restore_previous_window_on_hide = FALSE;
  }
}
#endif

static FlValue *build_rect_value(double x, double y, double width, double height)
{
  FlValue *rect = fl_value_new_map();
  fl_value_set_string_take(rect, "x", fl_value_new_float(x));
  fl_value_set_string_take(rect, "y", fl_value_new_float(y));
  fl_value_set_string_take(rect, "width", fl_value_new_float(width));
  fl_value_set_string_take(rect, "height", fl_value_new_float(height));
  return rect;
}

static gboolean encode_pixbuf_to_png_base64(GdkPixbuf *pixbuf, gchar **base64_out, gchar **error_out)
{
  gchar *png_buffer = nullptr;
  gsize png_size = 0;
  GError *save_error = nullptr;
  if (!gdk_pixbuf_save_to_buffer(pixbuf, &png_buffer, &png_size, "png", &save_error, nullptr))
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup(save_error != nullptr ? save_error->message : "Failed to encode screenshot PNG");
    }
    g_clear_error(&save_error);
    return FALSE;
  }

  *base64_out = g_base64_encode(reinterpret_cast<const guchar *>(png_buffer), png_size);
  g_free(png_buffer);
  return TRUE;
}

struct PortalRequestResponse
{
  GMainLoop *loop;
  guint response_code;
  GVariant *results;
  gboolean received;
};

struct PortalMonitorSnapshot
{
  std::string id;
  int x;
  int y;
  int width;
  int height;
};

static gboolean portal_timeout_cb(gpointer user_data)
{
  auto *response = static_cast<PortalRequestResponse *>(user_data);
  response->response_code = 2;
  g_main_loop_quit(response->loop);
  return G_SOURCE_REMOVE;
}

static void portal_response_cb(
    GDBusConnection *,
    const gchar *,
    const gchar *,
    const gchar *,
    const gchar *,
    GVariant *parameters,
    gpointer user_data)
{
  auto *response = static_cast<PortalRequestResponse *>(user_data);
  response->received = TRUE;
  g_variant_get(parameters, "(u@a{sv})", &response->response_code, &response->results);
  g_main_loop_quit(response->loop);
}

static gboolean wait_for_portal_response(
    GDBusConnection *connection,
    const gchar *request_path,
    PortalRequestResponse *response,
    gchar **error_out)
{
  response->loop = g_main_loop_new(nullptr, FALSE);
  response->response_code = 2;
  response->results = nullptr;
  response->received = FALSE;

  const guint timeout_source = g_timeout_add_seconds(12, portal_timeout_cb, response);
  const guint signal_id = g_dbus_connection_signal_subscribe(
      connection,
      "org.freedesktop.portal.Desktop",
      "org.freedesktop.portal.Request",
      "Response",
      request_path,
      nullptr,
      G_DBUS_SIGNAL_FLAGS_NONE,
      portal_response_cb,
      response,
      nullptr);

  g_main_loop_run(response->loop);

  g_source_remove(timeout_source);
  g_dbus_connection_signal_unsubscribe(connection, signal_id);
  g_main_loop_unref(response->loop);
  response->loop = nullptr;

  if (!response->received)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup("Timed out waiting for portal response");
    }
    return FALSE;
  }

  return TRUE;
}

static void clear_portal_response(PortalRequestResponse *response)
{
  if (response->results != nullptr)
  {
    g_variant_unref(response->results);
    response->results = nullptr;
  }
}

static gboolean call_portal_screenshot(
    GDBusConnection *connection,
    GdkPixbuf **pixbuf_out,
    gchar **error_out)
{
  GError *dbus_error = nullptr;
  GVariantBuilder options_builder;
  g_variant_builder_init(&options_builder, G_VARIANT_TYPE("a{sv}"));
  g_variant_builder_add(&options_builder, "{sv}", "interactive", g_variant_new_boolean(FALSE));
  gchar *handle_token = g_strdup_printf("wox_capture_%lld", static_cast<long long>(g_get_real_time()));
  g_variant_builder_add(&options_builder, "{sv}", "handle_token", g_variant_new_string(handle_token));

  GVariant *call_result = g_dbus_connection_call_sync(
      connection,
      "org.freedesktop.portal.Desktop",
      "/org/freedesktop/portal/desktop",
      "org.freedesktop.portal.Screenshot",
      "Screenshot",
      g_variant_new("(sa{sv})", "", &options_builder),
      G_VARIANT_TYPE("(o)"),
      G_DBUS_CALL_FLAGS_NONE,
      -1,
      nullptr,
      &dbus_error);
  g_free(handle_token);

  if (call_result == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup(dbus_error != nullptr ? dbus_error->message : "Failed to request portal screenshot");
    }
    g_clear_error(&dbus_error);
    return FALSE;
  }

  gchar *request_path = nullptr;
  g_variant_get(call_result, "(o)", &request_path);
  g_variant_unref(call_result);

  PortalRequestResponse response{};
  const gboolean response_ok = wait_for_portal_response(connection, request_path, &response, error_out);
  g_free(request_path);
  if (!response_ok)
  {
    return FALSE;
  }

  g_autofree gchar *uri = nullptr;
  if (response.response_code == 0 && response.results != nullptr)
  {
    GVariant *uri_value = g_variant_lookup_value(response.results, "uri", G_VARIANT_TYPE_STRING);
    if (uri_value != nullptr)
    {
      uri = g_strdup(g_variant_get_string(uri_value, nullptr));
      g_variant_unref(uri_value);
    }
  }

  if (response.response_code != 0 || uri == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup(response.response_code == 1 ? "Portal screenshot was cancelled" : "Portal screenshot request failed");
    }
    clear_portal_response(&response);
    return FALSE;
  }

  GError *file_error = nullptr;
  gchar *path = g_filename_from_uri(uri, nullptr, &file_error);
  if (path == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup(file_error != nullptr ? file_error->message : "Failed to read portal screenshot URI");
    }
    g_clear_error(&file_error);
    clear_portal_response(&response);
    return FALSE;
  }

  GdkPixbuf *pixbuf = gdk_pixbuf_new_from_file(path, &file_error);
  g_free(path);
  clear_portal_response(&response);
  if (pixbuf == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup(file_error != nullptr ? file_error->message : "Failed to load portal screenshot file");
    }
    g_clear_error(&file_error);
    return FALSE;
  }

  *pixbuf_out = pixbuf;
  return TRUE;
}

static void close_portal_session(GDBusConnection *connection, const gchar *session_handle)
{
  if (connection == nullptr || session_handle == nullptr)
  {
    return;
  }

  g_autoptr(GError) close_error = nullptr;
  GVariant *close_result = g_dbus_connection_call_sync(
      connection,
      "org.freedesktop.portal.Desktop",
      session_handle,
      "org.freedesktop.portal.Session",
      "Close",
      nullptr,
      nullptr,
      G_DBUS_CALL_FLAGS_NONE,
      -1,
      nullptr,
      &close_error);
  if (close_result != nullptr)
  {
    g_variant_unref(close_result);
  }
}

static gboolean lookup_portal_tuple(
    GVariant *dictionary,
    const gchar *key,
    gint *first,
    gint *second)
{
  GVariant *tuple = g_variant_lookup_value(dictionary, key, G_VARIANT_TYPE("(ii)"));
  if (tuple == nullptr)
  {
    return FALSE;
  }

  g_variant_get(tuple, "(ii)", first, second);
  g_variant_unref(tuple);
  return TRUE;
}

static gboolean capture_portal_monitor_metadata(
    GDBusConnection *connection,
    std::vector<PortalMonitorSnapshot> *monitors_out,
    gchar **error_out)
{
  GError *dbus_error = nullptr;
  gchar *session_handle = nullptr;

  GVariantBuilder create_options_builder;
  g_variant_builder_init(&create_options_builder, G_VARIANT_TYPE("a{sv}"));
  gchar *create_handle_token = g_strdup_printf("wox_screencast_create_%lld", static_cast<long long>(g_get_real_time()));
  gchar *session_handle_token = g_strdup_printf("wox_screencast_session_%lld", static_cast<long long>(g_get_real_time()));
  g_variant_builder_add(&create_options_builder, "{sv}", "handle_token", g_variant_new_string(create_handle_token));
  g_variant_builder_add(&create_options_builder, "{sv}", "session_handle_token", g_variant_new_string(session_handle_token));

  GVariant *create_call_result = g_dbus_connection_call_sync(
      connection,
      "org.freedesktop.portal.Desktop",
      "/org/freedesktop/portal/desktop",
      "org.freedesktop.portal.ScreenCast",
      "CreateSession",
      g_variant_new("(a{sv})", &create_options_builder),
      G_VARIANT_TYPE("(o)"),
      G_DBUS_CALL_FLAGS_NONE,
      -1,
      nullptr,
      &dbus_error);
  g_free(create_handle_token);
  g_free(session_handle_token);

  if (create_call_result == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup(dbus_error != nullptr ? dbus_error->message : "Failed to create portal ScreenCast session");
    }
    g_clear_error(&dbus_error);
    return FALSE;
  }

  gchar *create_request_path = nullptr;
  g_variant_get(create_call_result, "(o)", &create_request_path);
  g_variant_unref(create_call_result);

  PortalRequestResponse create_response{};
  const gboolean create_ok = wait_for_portal_response(connection, create_request_path, &create_response, error_out);
  g_free(create_request_path);
  if (!create_ok)
  {
    return FALSE;
  }

  if (create_response.response_code == 0 && create_response.results != nullptr)
  {
    GVariant *session_handle_value = g_variant_lookup_value(create_response.results, "session_handle", G_VARIANT_TYPE_STRING);
    if (session_handle_value != nullptr)
    {
      session_handle = g_strdup(g_variant_get_string(session_handle_value, nullptr));
      g_variant_unref(session_handle_value);
    }
  }
  clear_portal_response(&create_response);

  if (session_handle == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup("Portal ScreenCast session did not return a session handle");
    }
    return FALSE;
  }

  GVariantBuilder select_options_builder;
  g_variant_builder_init(&select_options_builder, G_VARIANT_TYPE("a{sv}"));
  gchar *select_handle_token = g_strdup_printf("wox_screencast_select_%lld", static_cast<long long>(g_get_real_time()));
  g_variant_builder_add(&select_options_builder, "{sv}", "handle_token", g_variant_new_string(select_handle_token));
  g_variant_builder_add(&select_options_builder, "{sv}", "types", g_variant_new_uint32(1));
  g_variant_builder_add(&select_options_builder, "{sv}", "multiple", g_variant_new_boolean(TRUE));
  g_variant_builder_add(&select_options_builder, "{sv}", "cursor_mode", g_variant_new_uint32(1));

  GVariant *select_call_result = g_dbus_connection_call_sync(
      connection,
      "org.freedesktop.portal.Desktop",
      "/org/freedesktop/portal/desktop",
      "org.freedesktop.portal.ScreenCast",
      "SelectSources",
      g_variant_new("(oa{sv})", session_handle, &select_options_builder),
      G_VARIANT_TYPE("(o)"),
      G_DBUS_CALL_FLAGS_NONE,
      -1,
      nullptr,
      &dbus_error);
  g_free(select_handle_token);

  if (select_call_result == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup(dbus_error != nullptr ? dbus_error->message : "Failed to select portal ScreenCast sources");
    }
    g_clear_error(&dbus_error);
    close_portal_session(connection, session_handle);
    g_free(session_handle);
    return FALSE;
  }

  gchar *select_request_path = nullptr;
  g_variant_get(select_call_result, "(o)", &select_request_path);
  g_variant_unref(select_call_result);

  PortalRequestResponse select_response{};
  const gboolean select_ok = wait_for_portal_response(connection, select_request_path, &select_response, error_out);
  g_free(select_request_path);
  if (!select_ok)
  {
    close_portal_session(connection, session_handle);
    g_free(session_handle);
    return FALSE;
  }

  const gboolean select_succeeded = select_response.response_code == 0;
  clear_portal_response(&select_response);
  if (!select_succeeded)
  {
    if (error_out != nullptr && *error_out == nullptr)
    {
      *error_out = g_strdup("Portal ScreenCast source selection was cancelled");
    }
    close_portal_session(connection, session_handle);
    g_free(session_handle);
    return FALSE;
  }

  GVariantBuilder start_options_builder;
  g_variant_builder_init(&start_options_builder, G_VARIANT_TYPE("a{sv}"));
  gchar *start_handle_token = g_strdup_printf("wox_screencast_start_%lld", static_cast<long long>(g_get_real_time()));
  g_variant_builder_add(&start_options_builder, "{sv}", "handle_token", g_variant_new_string(start_handle_token));

  GVariant *start_call_result = g_dbus_connection_call_sync(
      connection,
      "org.freedesktop.portal.Desktop",
      "/org/freedesktop/portal/desktop",
      "org.freedesktop.portal.ScreenCast",
      "Start",
      g_variant_new("(osa{sv})", session_handle, "", &start_options_builder),
      G_VARIANT_TYPE("(o)"),
      G_DBUS_CALL_FLAGS_NONE,
      -1,
      nullptr,
      &dbus_error);
  g_free(start_handle_token);

  if (start_call_result == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup(dbus_error != nullptr ? dbus_error->message : "Failed to start portal ScreenCast session");
    }
    g_clear_error(&dbus_error);
    close_portal_session(connection, session_handle);
    g_free(session_handle);
    return FALSE;
  }

  gchar *start_request_path = nullptr;
  g_variant_get(start_call_result, "(o)", &start_request_path);
  g_variant_unref(start_call_result);

  PortalRequestResponse start_response{};
  const gboolean start_ok = wait_for_portal_response(connection, start_request_path, &start_response, error_out);
  g_free(start_request_path);
  if (!start_ok)
  {
    close_portal_session(connection, session_handle);
    g_free(session_handle);
    return FALSE;
  }

  if (start_response.response_code != 0 || start_response.results == nullptr)
  {
    if (error_out != nullptr && *error_out == nullptr)
    {
      *error_out = g_strdup(start_response.response_code == 1 ? "Portal ScreenCast session was cancelled" : "Portal ScreenCast session failed");
    }
    clear_portal_response(&start_response);
    close_portal_session(connection, session_handle);
    g_free(session_handle);
    return FALSE;
  }

  GVariant *streams_value = g_variant_lookup_value(start_response.results, "streams", G_VARIANT_TYPE("a(ua{sv})"));
  if (streams_value == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup("Portal ScreenCast session did not return monitor streams");
    }
    clear_portal_response(&start_response);
    close_portal_session(connection, session_handle);
    g_free(session_handle);
    return FALSE;
  }

  GVariantIter streams_iter;
  g_variant_iter_init(&streams_iter, streams_value);
  GVariant *stream_entry = nullptr;
  while ((stream_entry = g_variant_iter_next_value(&streams_iter)) != nullptr)
  {
    guint32 node_id = 0;
    GVariant *stream_properties = nullptr;
    g_variant_get(stream_entry, "(u@a{sv})", &node_id, &stream_properties);

    guint32 source_type = 0;
    g_variant_lookup(stream_properties, "source_type", "u", &source_type);
    gint x = 0;
    gint y = 0;
    gint width = 0;
    gint height = 0;

    if (source_type == 1 &&
        lookup_portal_tuple(stream_properties, "position", &x, &y) &&
        lookup_portal_tuple(stream_properties, "size", &width, &height) &&
        width > 0 &&
        height > 0)
    {
      GVariant *id_value = g_variant_lookup_value(stream_properties, "id", G_VARIANT_TYPE_STRING);
      std::string display_id = id_value != nullptr ? g_variant_get_string(id_value, nullptr) : "";
      if (id_value != nullptr)
      {
        g_variant_unref(id_value);
      }
      if (display_id.empty())
      {
        display_id = "portal-monitor-" + std::to_string(node_id);
      }

      monitors_out->push_back(
          PortalMonitorSnapshot{
              display_id,
              x,
              y,
              width,
              height,
          });
    }

    if (stream_properties != nullptr)
    {
      g_variant_unref(stream_properties);
    }
    g_variant_unref(stream_entry);
  }

  g_variant_unref(streams_value);
  clear_portal_response(&start_response);
  close_portal_session(connection, session_handle);
  g_free(session_handle);

  if (monitors_out->empty())
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup("Portal ScreenCast session did not expose any monitor streams");
    }
    return FALSE;
  }

  return TRUE;
}

static gboolean capture_portal_monitor_snapshots(FlValue **snapshots_out, gchar **error_out)
{
  GError *dbus_error = nullptr;
  GDBusConnection *connection = g_bus_get_sync(G_BUS_TYPE_SESSION, nullptr, &dbus_error);
  if (connection == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup(dbus_error != nullptr ? dbus_error->message : "Failed to connect to portal session bus");
    }
    g_clear_error(&dbus_error);
    return FALSE;
  }

  std::vector<PortalMonitorSnapshot> monitors;
  const gboolean metadata_ok = capture_portal_monitor_metadata(connection, &monitors, error_out);
  if (!metadata_ok)
  {
    g_object_unref(connection);
    return FALSE;
  }

  GdkPixbuf *desktop_pixbuf = nullptr;
  const gboolean screenshot_ok = call_portal_screenshot(connection, &desktop_pixbuf, error_out);
  g_object_unref(connection);
  if (!screenshot_ok)
  {
    return FALSE;
  }

  int union_left = monitors.front().x;
  int union_top = monitors.front().y;
  int union_right = monitors.front().x + monitors.front().width;
  int union_bottom = monitors.front().y + monitors.front().height;
  for (size_t index = 1; index < monitors.size(); ++index)
  {
    const auto &monitor = monitors[index];
    union_left = MIN(union_left, monitor.x);
    union_top = MIN(union_top, monitor.y);
    union_right = MAX(union_right, monitor.x + monitor.width);
    union_bottom = MAX(union_bottom, monitor.y + monitor.height);
  }

  const int union_width = union_right - union_left;
  const int union_height = union_bottom - union_top;
  const int desktop_pixel_width = gdk_pixbuf_get_width(desktop_pixbuf);
  const int desktop_pixel_height = gdk_pixbuf_get_height(desktop_pixbuf);
  const double scale_x = union_width > 0 ? static_cast<double>(desktop_pixel_width) / union_width : 1.0;
  const double scale_y = union_height > 0 ? static_cast<double>(desktop_pixel_height) / union_height : 1.0;

  g_autoptr(FlValue) snapshots = fl_value_new_list();
  for (const auto &monitor : monitors)
  {
    int crop_x = static_cast<int>(round((monitor.x - union_left) * scale_x));
    int crop_y = static_cast<int>(round((monitor.y - union_top) * scale_y));
    int crop_width = static_cast<int>(round(monitor.width * scale_x));
    int crop_height = static_cast<int>(round(monitor.height * scale_y));

    crop_x = CLAMP(crop_x, 0, MAX(0, desktop_pixel_width - 1));
    crop_y = CLAMP(crop_y, 0, MAX(0, desktop_pixel_height - 1));
    crop_width = CLAMP(crop_width, 1, desktop_pixel_width - crop_x);
    crop_height = CLAMP(crop_height, 1, desktop_pixel_height - crop_y);

    // Wayland does not expose compositor pixels directly to GTK. We use the ScreenCast portal to
    // obtain monitor-sized compositor coordinates, then slice the portal desktop screenshot into
    // one image per monitor so Flutter still receives the multi-display surfaces it needs.
    GdkPixbuf *monitor_pixbuf = gdk_pixbuf_new_subpixbuf(
        desktop_pixbuf,
        crop_x,
        crop_y,
        crop_width,
        crop_height);
    if (monitor_pixbuf == nullptr)
    {
      if (error_out != nullptr)
      {
        *error_out = g_strdup("Failed to crop portal monitor snapshot");
      }
      g_object_unref(desktop_pixbuf);
      return FALSE;
    }

    gchar *image_base64 = nullptr;
    gchar *encode_error = nullptr;
    if (!encode_pixbuf_to_png_base64(monitor_pixbuf, &image_base64, &encode_error))
    {
      if (error_out != nullptr)
      {
        *error_out = encode_error != nullptr ? encode_error : g_strdup("Failed to encode portal monitor snapshot");
      }
      g_free(encode_error);
      g_object_unref(monitor_pixbuf);
      g_object_unref(desktop_pixbuf);
      return FALSE;
    }

    FlValue *snapshot = fl_value_new_map();
    fl_value_set_string_take(snapshot, "displayId", fl_value_new_string(monitor.id.c_str()));
    fl_value_set_string_take(snapshot, "logicalBounds", build_rect_value(monitor.x, monitor.y, monitor.width, monitor.height));
    fl_value_set_string_take(snapshot, "pixelBounds", build_rect_value(crop_x, crop_y, crop_width, crop_height));
    fl_value_set_string_take(snapshot, "scale", fl_value_new_float(monitor.width > 0 ? static_cast<double>(crop_width) / monitor.width : scale_x));
    fl_value_set_string_take(snapshot, "rotation", fl_value_new_int(0));
    fl_value_set_string_take(snapshot, "imageBytesBase64", fl_value_new_string(image_base64));
    g_free(image_base64);
    g_object_unref(monitor_pixbuf);
    fl_value_append_take(snapshots, snapshot);
  }

  g_object_unref(desktop_pixbuf);
  *snapshots_out = g_steal_pointer(&snapshots);
  return TRUE;
}

static gboolean capture_portal_desktop_snapshot(FlValue **snapshot_out, gchar **error_out)
{
  GError *dbus_error = nullptr;
  GDBusConnection *connection = g_bus_get_sync(G_BUS_TYPE_SESSION, nullptr, &dbus_error);
  if (connection == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup(dbus_error != nullptr ? dbus_error->message : "Failed to connect to portal session bus");
    }
    g_clear_error(&dbus_error);
    return FALSE;
  }

  GdkDisplay *display = gdk_display_get_default();
  if (display == nullptr)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup("Failed to access GDK display");
    }
    g_object_unref(connection);
    return FALSE;
  }

  int monitor_count = gdk_display_get_n_monitors(display);
  if (monitor_count <= 0)
  {
    if (error_out != nullptr)
    {
      *error_out = g_strdup("No monitors are available for capture");
    }
    g_object_unref(connection);
    return FALSE;
  }

  GdkRectangle logical_union{};
  gboolean union_initialized = FALSE;
  for (int index = 0; index < monitor_count; ++index)
  {
    GdkMonitor *monitor = gdk_display_get_monitor(display, index);
    if (monitor == nullptr)
    {
      continue;
    }

    GdkRectangle geometry{};
    gdk_monitor_get_geometry(monitor, &geometry);
    if (!union_initialized)
    {
      logical_union = geometry;
      union_initialized = TRUE;
      continue;
    }

    const int left = MIN(logical_union.x, geometry.x);
    const int top = MIN(logical_union.y, geometry.y);
    const int right = MAX(logical_union.x + logical_union.width, geometry.x + geometry.width);
    const int bottom = MAX(logical_union.y + logical_union.height, geometry.y + geometry.height);
    logical_union.x = left;
    logical_union.y = top;
    logical_union.width = right - left;
    logical_union.height = bottom - top;
  }

  GdkPixbuf *pixbuf = nullptr;
  const gboolean screenshot_ok = call_portal_screenshot(connection, &pixbuf, error_out);
  g_object_unref(connection);
  if (!screenshot_ok)
  {
    return FALSE;
  }

  gchar *image_base64 = nullptr;
  gchar *encode_error = nullptr;
  if (!encode_pixbuf_to_png_base64(pixbuf, &image_base64, &encode_error))
  {
    if (error_out != nullptr)
    {
      *error_out = encode_error != nullptr ? encode_error : g_strdup("Failed to encode portal screenshot");
    }
    g_free(encode_error);
    g_object_unref(pixbuf);
    return FALSE;
  }

  const int pixel_width = gdk_pixbuf_get_width(pixbuf);
  const int pixel_height = gdk_pixbuf_get_height(pixbuf);
  g_object_unref(pixbuf);

  const double scale_x = logical_union.width > 0 ? static_cast<double>(pixel_width) / logical_union.width : 1.0;
  const double scale_y = logical_union.height > 0 ? static_cast<double>(pixel_height) / logical_union.height : 1.0;

  FlValue *snapshot = fl_value_new_map();
  fl_value_set_string_take(snapshot, "displayId", fl_value_new_string("portal:desktop"));
  fl_value_set_string_take(snapshot, "logicalBounds", build_rect_value(logical_union.x, logical_union.y, logical_union.width, logical_union.height));
  fl_value_set_string_take(snapshot, "pixelBounds", build_rect_value(logical_union.x * scale_x, logical_union.y * scale_y, pixel_width, pixel_height));
  fl_value_set_string_take(snapshot, "scale", fl_value_new_float(scale_x));
  fl_value_set_string_take(snapshot, "rotation", fl_value_new_int(0));
  fl_value_set_string_take(snapshot, "imageBytesBase64", fl_value_new_string(image_base64));
  g_free(image_base64);

  *snapshot_out = snapshot;
  return TRUE;
}

#ifdef GDK_WINDOWING_X11
static KeySym parse_x11_key_sym(const std::string &key)
{
  if (key == "alt")
    return XK_Alt_L;
  if (key == "control")
    return XK_Control_L;
  if (key == "shift")
    return XK_Shift_L;
  if (key == "meta")
    return XK_Super_L;
  if (key == "escape")
    return XK_Escape;
  if (key == "enter")
    return XK_Return;
  if (key == "tab")
    return XK_Tab;
  if (key == "space")
    return XK_space;
  if (key == "arrowUp")
    return XK_Up;
  if (key == "arrowDown")
    return XK_Down;
  if (key == "arrowLeft")
    return XK_Left;
  if (key == "arrowRight")
    return XK_Right;

  if (key.size() == 1)
  {
    return XStringToKeysym(key.c_str());
  }

  return NoSymbol;
}

static gboolean send_x11_key_event(GtkWindow *window, const std::string &key,
                                   bool is_press)
{
  Display *display = get_x11_display(window);
  if (display == nullptr)
  {
    return FALSE;
  }

  KeySym key_sym = parse_x11_key_sym(key);
  if (key_sym == NoSymbol)
  {
    return FALSE;
  }

  KeyCode key_code = XKeysymToKeycode(display, key_sym);
  if (key_code == 0)
  {
    return FALSE;
  }

  XTestFakeKeyEvent(display, key_code, is_press ? True : False, CurrentTime);
  XFlush(display);
  return TRUE;
}

static gboolean move_x11_mouse(GtkWindow *window, int x, int y)
{
  Display *display = get_x11_display(window);
  if (display == nullptr)
  {
    return FALSE;
  }

  XWarpPointer(display, None, DefaultRootWindow(display), 0, 0, 0, 0, x, y);
  XFlush(display);
  return TRUE;
}

static int parse_x11_mouse_button(const std::string &button)
{
  if (button == "left")
    return Button1;
  if (button == "middle")
    return Button2;
  if (button == "right")
    return Button3;
  return 0;
}

static gboolean send_x11_mouse_button(GtkWindow *window, const std::string &button,
                                      bool is_press)
{
  Display *display = get_x11_display(window);
  if (display == nullptr)
  {
    return FALSE;
  }

  int button_id = parse_x11_mouse_button(button);
  if (button_id == 0)
  {
    return FALSE;
  }

  XTestFakeButtonEvent(display, button_id, is_press ? True : False, CurrentTime);
  XFlush(display);
  return TRUE;
}
#endif

// Method channel handler
static void method_call_cb(FlMethodChannel *channel, FlMethodCall *method_call,
                           gpointer user_data)
{
  MyApplication *self = MY_APPLICATION(user_data);
  GtkWindow *window = self->window;
  const gchar *method = fl_method_call_get_name(method_call);
  FlValue *args = fl_method_call_get_args(method_call);
  g_autoptr(FlMethodResponse) response = nullptr;

  if (strcmp(method, "captureAllDisplays") == 0)
  {
#ifdef GDK_WINDOWING_X11
    GdkDisplay *display = gtk_widget_get_display(GTK_WIDGET(window));
    if (display != nullptr && GDK_IS_X11_DISPLAY(display))
    {
      GdkWindow *root_window = gdk_get_default_root_window();
      if (root_window == nullptr)
      {
        response = FL_METHOD_RESPONSE(fl_method_error_response_new("CAPTURE_ERROR", "Failed to access X11 root window", nullptr));
      }
      else
      {
        g_autoptr(FlValue) snapshots = fl_value_new_list();
        const int monitor_count = gdk_display_get_n_monitors(display);
        gchar *capture_error = nullptr;

        for (int index = 0; index < monitor_count; ++index)
        {
          GdkMonitor *monitor = gdk_display_get_monitor(display, index);
          if (monitor == nullptr)
          {
            continue;
          }

          GdkRectangle geometry{};
          gdk_monitor_get_geometry(monitor, &geometry);
          GdkPixbuf *pixbuf = gdk_pixbuf_get_from_window(root_window, geometry.x, geometry.y, geometry.width, geometry.height);
          if (pixbuf == nullptr)
          {
            capture_error = g_strdup("Failed to capture X11 monitor");
            break;
          }

          gchar *image_base64 = nullptr;
          gchar *encode_error = nullptr;
          if (!encode_pixbuf_to_png_base64(pixbuf, &image_base64, &encode_error))
          {
            capture_error = encode_error != nullptr ? encode_error : g_strdup("Failed to encode X11 monitor image");
            g_object_unref(pixbuf);
            break;
          }

          const int pixel_width = gdk_pixbuf_get_width(pixbuf);
          const int pixel_height = gdk_pixbuf_get_height(pixbuf);
          g_object_unref(pixbuf);

          const int scale = gdk_monitor_get_scale_factor(monitor);
          gchar *display_id = g_strdup_printf("x11-monitor-%d", index);
          FlValue *snapshot = fl_value_new_map();
          fl_value_set_string_take(snapshot, "displayId", fl_value_new_string(display_id));
          fl_value_set_string_take(snapshot, "logicalBounds", build_rect_value(geometry.x, geometry.y, geometry.width, geometry.height));
          fl_value_set_string_take(snapshot, "pixelBounds", build_rect_value(geometry.x * scale, geometry.y * scale, pixel_width, pixel_height));
          fl_value_set_string_take(snapshot, "scale", fl_value_new_float(scale));
          fl_value_set_string_take(snapshot, "rotation", fl_value_new_int(0));
          fl_value_set_string_take(snapshot, "imageBytesBase64", fl_value_new_string(image_base64));
          g_free(image_base64);
          g_free(display_id);
          fl_value_append_take(snapshots, snapshot);
        }

        if (capture_error != nullptr)
        {
          response = FL_METHOD_RESPONSE(fl_method_error_response_new("CAPTURE_ERROR", capture_error, nullptr));
          g_free(capture_error);
        }
        else
        {
          response = FL_METHOD_RESPONSE(fl_method_success_response_new(snapshots));
        }
      }
    }
    else
#endif
    {
      FlValue *portal_snapshots = nullptr;
      gchar *capture_error = nullptr;
      if (capture_portal_monitor_snapshots(&portal_snapshots, &capture_error))
      {
        response = FL_METHOD_RESPONSE(fl_method_success_response_new(portal_snapshots));
      }
      else
      {
        g_free(capture_error);

        // Some desktops do not expose ScreenCast on the active portal backend. Keep the older
        // single-desktop screenshot fallback so capture remains available even when Wayland cannot
        // provide per-monitor metadata for the Flutter workspace.
        FlValue *snapshot = nullptr;
        gchar *desktop_capture_error = nullptr;
        if (capture_portal_desktop_snapshot(&snapshot, &desktop_capture_error))
        {
          g_autoptr(FlValue) snapshots = fl_value_new_list();
          fl_value_append_take(snapshots, snapshot);
          response = FL_METHOD_RESPONSE(fl_method_success_response_new(snapshots));
        }
        else
        {
          response = FL_METHOD_RESPONSE(fl_method_error_response_new("CAPTURE_ERROR", desktop_capture_error != nullptr ? desktop_capture_error : "Portal screenshot capture failed", nullptr));
          g_free(desktop_capture_error);
        }
      }
    }
  }
  else if (strcmp(method, "inputKeyDown") == 0 || strcmp(method, "inputKeyUp") == 0)
  {
#ifdef GDK_WINDOWING_X11
    if (fl_value_get_type(args) == FL_VALUE_TYPE_MAP)
    {
      FlValue *key_value = fl_value_lookup_string(args, "key");
      if (key_value != nullptr && fl_value_get_type(key_value) == FL_VALUE_TYPE_STRING)
      {
        std::string key = fl_value_get_string(key_value);
        gboolean handled = send_x11_key_event(window, key, strcmp(method, "inputKeyDown") == 0);
        if (handled)
        {
          response = FL_METHOD_RESPONSE(fl_method_success_response_new(fl_value_new_null()));
        }
        else
        {
          response = FL_METHOD_RESPONSE(fl_method_error_response_new("INPUT_ERROR", "Failed to send X11 keyboard event", nullptr));
        }
      }
      else
      {
        response = FL_METHOD_RESPONSE(fl_method_error_response_new("INVALID_ARGS", "Missing key for keyboard input", nullptr));
      }
    }
    else
    {
      response = FL_METHOD_RESPONSE(fl_method_error_response_new("INVALID_ARGS", "Missing arguments for keyboard input", nullptr));
    }
#else
    response = FL_METHOD_RESPONSE(fl_method_error_response_new("UNSUPPORTED", "System keyboard input is only implemented for X11 Linux sessions", nullptr));
#endif
  }
  else if (strcmp(method, "inputMouseMove") == 0)
  {
#ifdef GDK_WINDOWING_X11
    if (fl_value_get_type(args) == FL_VALUE_TYPE_MAP)
    {
      FlValue *x_value = fl_value_lookup_string(args, "x");
      FlValue *y_value = fl_value_lookup_string(args, "y");
      if (x_value != nullptr && y_value != nullptr && fl_value_get_type(x_value) == FL_VALUE_TYPE_FLOAT && fl_value_get_type(y_value) == FL_VALUE_TYPE_FLOAT)
      {
        gboolean handled = move_x11_mouse(window, (int)round(fl_value_get_float(x_value)), (int)round(fl_value_get_float(y_value)));
        if (handled)
        {
          response = FL_METHOD_RESPONSE(fl_method_success_response_new(fl_value_new_null()));
        }
        else
        {
          response = FL_METHOD_RESPONSE(fl_method_error_response_new("INPUT_ERROR", "Failed to move X11 mouse cursor", nullptr));
        }
      }
      else
      {
        response = FL_METHOD_RESPONSE(fl_method_error_response_new("INVALID_ARGS", "Missing coordinates for mouse move", nullptr));
      }
    }
    else
    {
      response = FL_METHOD_RESPONSE(fl_method_error_response_new("INVALID_ARGS", "Missing arguments for mouse move", nullptr));
    }
#else
    response = FL_METHOD_RESPONSE(fl_method_error_response_new("UNSUPPORTED", "System mouse input is only implemented for X11 Linux sessions", nullptr));
#endif
  }
  else if (strcmp(method, "inputMouseDown") == 0 || strcmp(method, "inputMouseUp") == 0)
  {
#ifdef GDK_WINDOWING_X11
    if (fl_value_get_type(args) == FL_VALUE_TYPE_MAP)
    {
      FlValue *button_value = fl_value_lookup_string(args, "button");
      if (button_value != nullptr && fl_value_get_type(button_value) == FL_VALUE_TYPE_STRING)
      {
        std::string button = fl_value_get_string(button_value);
        gboolean handled = send_x11_mouse_button(window, button, strcmp(method, "inputMouseDown") == 0);
        if (handled)
        {
          response = FL_METHOD_RESPONSE(fl_method_success_response_new(fl_value_new_null()));
        }
        else
        {
          response = FL_METHOD_RESPONSE(fl_method_error_response_new("INPUT_ERROR", "Failed to send X11 mouse button event", nullptr));
        }
      }
      else
      {
        response = FL_METHOD_RESPONSE(fl_method_error_response_new("INVALID_ARGS", "Missing mouse button", nullptr));
      }
    }
    else
    {
      response = FL_METHOD_RESPONSE(fl_method_error_response_new("INVALID_ARGS", "Missing arguments for mouse button input", nullptr));
    }
#else
    response = FL_METHOD_RESPONSE(fl_method_error_response_new("UNSUPPORTED", "System mouse input is only implemented for X11 Linux sessions", nullptr));
#endif
  }
  else if (strcmp(method, "setSize") == 0)
  {
    if (fl_value_get_type(args) == FL_VALUE_TYPE_MAP)
    {
      double width = fl_value_get_float(fl_value_lookup_string(args, "width"));
      double height =
          fl_value_get_float(fl_value_lookup_string(args, "height"));
      gtk_window_resize(window, (int)width, (int)height);
      response = FL_METHOD_RESPONSE(
          fl_method_success_response_new(fl_value_new_null()));
    }
  }
  else if (strcmp(method, "setBounds") == 0)
  {
    if (fl_value_get_type(args) == FL_VALUE_TYPE_MAP)
    {
      double x = fl_value_get_float(fl_value_lookup_string(args, "x"));
      double y = fl_value_get_float(fl_value_lookup_string(args, "y"));
      double width = fl_value_get_float(fl_value_lookup_string(args, "width"));
      double height = fl_value_get_float(fl_value_lookup_string(args, "height"));
      GdkWindow *gdk_window = gtk_widget_get_window(GTK_WIDGET(window));
      if (gdk_window != nullptr)
      {
        gdk_window_move_resize(gdk_window, (int)x, (int)y, (int)width, (int)height);
      }
      else
      {
        gtk_window_move(window, (int)x, (int)y);
        gtk_window_resize(window, (int)width, (int)height);
      }
      response = FL_METHOD_RESPONSE(
          fl_method_success_response_new(fl_value_new_null()));
    }
  }
  else if (strcmp(method, "getPosition") == 0)
  {
    int x, y;
    gtk_window_get_position(window, &x, &y);
    g_autoptr(FlValue) result = fl_value_new_map();
    fl_value_set_string_take(result, "x", fl_value_new_int(x));
    fl_value_set_string_take(result, "y", fl_value_new_int(y));
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(result));
  }
  else if (strcmp(method, "getSize") == 0)
  {
    int w, h;
    gtk_window_get_size(window, &w, &h);
    g_autoptr(FlValue) result = fl_value_new_map();
    fl_value_set_string_take(result, "width", fl_value_new_int(w));
    fl_value_set_string_take(result, "height", fl_value_new_int(h));
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(result));
  }
  else if (strcmp(method, "setPosition") == 0)
  {
    if (fl_value_get_type(args) == FL_VALUE_TYPE_MAP)
    {
      double x = fl_value_get_float(fl_value_lookup_string(args, "x"));
      double y = fl_value_get_float(fl_value_lookup_string(args, "y"));
      gtk_window_move(window, (int)x, (int)y);
      log("FLUTTER: setPosition, x: %f, y: %f", x, y);
      response = FL_METHOD_RESPONSE(
          fl_method_success_response_new(fl_value_new_null()));
    }
  }
  else if (strcmp(method, "center") == 0)
  {
    // 获取窗口尺寸，优先使用传入的参数
    int window_width, window_height;

    if (fl_value_get_type(args) == FL_VALUE_TYPE_MAP)
    {
      FlValue *width_value = fl_value_lookup_string(args, "width");
      FlValue *height_value = fl_value_lookup_string(args, "height");

      if (width_value != nullptr &&
          fl_value_get_type(width_value) == FL_VALUE_TYPE_FLOAT &&
          height_value != nullptr &&
          fl_value_get_type(height_value) == FL_VALUE_TYPE_FLOAT)
      {
        // 使用传入的尺寸
        window_width = (int)fl_value_get_float(width_value);
        window_height = (int)fl_value_get_float(height_value);

        // 调整窗口大小
        gtk_window_resize(window, window_width, window_height);
      }
      else
      {
        // 使用当前窗口尺寸
        gtk_window_get_size(window, &window_width, &window_height);
      }
    }
    else
    {
      // 使用当前窗口尺寸
      gtk_window_get_size(window, &window_width, &window_height);
    }

    // 获取鼠标所在的屏幕
    GdkDisplay *display = gtk_widget_get_display(GTK_WIDGET(window));
    GdkSeat *seat = gdk_display_get_default_seat(display);
    GdkDevice *pointer = gdk_seat_get_pointer(seat);

    int mouse_x, mouse_y;
    gdk_device_get_position(pointer, NULL, &mouse_x, &mouse_y);

    GdkMonitor *monitor = gdk_display_get_monitor_at_point(display, mouse_x, mouse_y);
    if (monitor == NULL)
    {
      monitor = gdk_display_get_primary_monitor(display);
    }

    GdkRectangle workarea;
    gdk_monitor_get_workarea(monitor, &workarea);

    // 计算居中位置
    int x = workarea.x + (workarea.width - window_width) / 2;
    int y = workarea.y + (workarea.height - window_height) / 2;

    log("FLUTTER: center, window to %d,%d on monitor at %d,%d", x, y, workarea.x, workarea.y);

    // 设置窗口位置
    gtk_window_move(window, x, y);

    response =
        FL_METHOD_RESPONSE(fl_method_success_response_new(fl_value_new_null()));
  }
  else if (strcmp(method, "show") == 0)
  {
    save_previous_active_window(self);
    gtk_widget_show(GTK_WIDGET(window));
    response =
        FL_METHOD_RESPONSE(fl_method_success_response_new(fl_value_new_null()));
  }
  else if (strcmp(method, "hide") == 0)
  {
    gboolean is_active = gtk_window_is_active(window);
    gboolean should_restore_previous_window = self->restore_previous_window_on_hide;
    gtk_widget_hide(GTK_WIDGET(window));
    if (is_active && should_restore_previous_window)
    {
      restore_previous_active_window(self);
    }
    else
    {
      self->previous_active_window = 0;
    }
    self->restore_previous_window_on_hide = FALSE;
    response =
        FL_METHOD_RESPONSE(fl_method_success_response_new(fl_value_new_null()));
  }
  else if (strcmp(method, "focus") == 0)
  {
    log("FLUTTER: focus - attempting to focus window");
    save_previous_active_window(self);

    GdkWindow *gdk_window = gtk_widget_get_window(GTK_WIDGET(window));
    if (gdk_window)
    {
      gdk_window_raise(gdk_window);
      gdk_window_focus(gdk_window, GDK_CURRENT_TIME);

#ifdef GDK_WINDOWING_X11
      if (GDK_IS_X11_WINDOW(gdk_window))
      {
        Display *display =
            GDK_DISPLAY_XDISPLAY(gdk_window_get_display(gdk_window));
        Window xid = GDK_WINDOW_XID(gdk_window);

        log("FLUTTER: focus - using X11 specific methods");

        // 更安全的X11代码实现
        XRaiseWindow(display, xid);

        // 使用简化的_NET_ACTIVE_WINDOW消息
        Atom net_active_window =
            XInternAtom(display, "_NET_ACTIVE_WINDOW", False);
        if (net_active_window != None)
        {
          XEvent xev;
          memset(&xev, 0, sizeof(xev));
          xev.type = ClientMessage;
          xev.xclient.type = ClientMessage;
          xev.xclient.window = xid;
          xev.xclient.message_type = net_active_window;
          xev.xclient.format = 32;
          xev.xclient.data.l[0] = 2; // 来源指示: 2 = 来自应用程序的请求
          xev.xclient.data.l[1] = CurrentTime;

          XSendEvent(display, DefaultRootWindow(display), False,
                     SubstructureRedirectMask | SubstructureNotifyMask, &xev);

          XFlush(display);
        }
      }
#endif
    }

    // 使用GTK的标准方法
    gtk_window_present(window);
    gtk_widget_grab_focus(GTK_WIDGET(window));
    log("FLUTTER: focus - all focus operations completed");
    response =
        FL_METHOD_RESPONSE(fl_method_success_response_new(fl_value_new_null()));
  }
  else if (strcmp(method, "isVisible") == 0)
  {
    gboolean visible = gtk_widget_get_visible(GTK_WIDGET(window));
    response = FL_METHOD_RESPONSE(
        fl_method_success_response_new(fl_value_new_bool(visible)));
  }
  else if (strcmp(method, "setAlwaysOnTop") == 0)
  {
    gboolean always_on_top = FALSE;
    if (fl_value_get_type(args) == FL_VALUE_TYPE_BOOL)
    {
      always_on_top = fl_value_get_bool(args);
    }
    gtk_window_set_keep_above(window, always_on_top);
    response =
        FL_METHOD_RESPONSE(fl_method_success_response_new(fl_value_new_null()));
  }
  else if (strcmp(method, "waitUntilReadyToShow") == 0)
  {
    response =
        FL_METHOD_RESPONSE(fl_method_success_response_new(fl_value_new_null()));
  }
  else
  {
    response = FL_METHOD_RESPONSE(fl_method_not_implemented_response_new());
  }

  g_autoptr(GError) error = nullptr;
  if (!fl_method_call_respond(method_call, response, &error))
  {
    g_warning("Failed to send response: %s", error->message);
  }
}

// Implements GApplication::activate.
static void my_application_activate(GApplication *application)
{
  MyApplication *self = MY_APPLICATION(application);
  GtkWindow *window =
      GTK_WINDOW(gtk_application_window_new(GTK_APPLICATION(application)));

  // Store window reference in application instance
  self->window = window;

  // Remove window decorations (titlebar)
  gtk_window_set_decorated(window, FALSE);

  // Use a header bar when running in GNOME as this is the common style used
  // by applications and is the setup most users will be using (e.g. Ubuntu
  // desktop).
  // If running on X and not using GNOME then just use a traditional title bar
  // in case the window manager does more exotic layout, e.g. tiling.
  // If running on Wayland assume the header bar will work (may need changing
  // if future cases occur).
  gboolean use_header_bar = TRUE;
#ifdef GDK_WINDOWING_X11
  GdkScreen *screen = gtk_window_get_screen(window);
  if (GDK_IS_X11_SCREEN(screen))
  {
    const gchar *wm_name = gdk_x11_screen_get_window_manager_name(screen);
    if (g_strcmp0(wm_name, "GNOME Shell") != 0)
    {
      use_header_bar = FALSE;
    }
  }
#endif
  if (use_header_bar)
  {
    GtkHeaderBar *header_bar = GTK_HEADER_BAR(gtk_header_bar_new());
    gtk_widget_show(GTK_WIDGET(header_bar));
    gtk_header_bar_set_title(header_bar, "Wox");
    gtk_header_bar_set_show_close_button(header_bar, TRUE);
    gtk_window_set_titlebar(window, GTK_WIDGET(header_bar));
  }
  else
  {
    gtk_window_set_title(window, "Wox");
  }

  gtk_window_set_default_size(window, 1280, 720);

  // Prevent notifications and taskbar entries
  gtk_window_set_skip_taskbar_hint(window, TRUE);
  gtk_window_set_type_hint(window, GDK_WINDOW_TYPE_HINT_UTILITY);
  gtk_window_set_keep_above(window, TRUE);

  g_autoptr(FlDartProject) project = fl_dart_project_new();
  fl_dart_project_set_dart_entrypoint_arguments(
      project, self->dart_entrypoint_arguments);

  // By default the window background is transparent(not acrylic), which is not
  // what we want so following code is to make the window background opaque
  GtkBox *box = GTK_BOX(gtk_box_new(GTK_ORIENTATION_VERTICAL, 0));
  gtk_widget_show(GTK_WIDGET(box));
  const gchar *css = "box { background-color: #FFFFFF; }";
  GtkCssProvider *provider = gtk_css_provider_new();
  gtk_css_provider_load_from_data(provider, css, -1, nullptr);
  GtkStyleContext *context = gtk_widget_get_style_context(GTK_WIDGET(box));
  gtk_style_context_add_class(context, "box");
  gtk_style_context_add_provider(context, GTK_STYLE_PROVIDER(provider),
                                 GTK_STYLE_PROVIDER_PRIORITY_USER);

  g_object_unref(provider);
  gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(box));

  FlView *view = fl_view_new(project);
  gtk_widget_show(GTK_WIDGET(view));
  gtk_container_add(GTK_CONTAINER(box), GTK_WIDGET(view));
  fl_register_plugins(FL_PLUGIN_REGISTRY(view));

  // Set up method channel for window management
  g_autoptr(FlStandardMethodCodec) codec = fl_standard_method_codec_new();
  g_autoptr(FlMethodChannel) channel = fl_method_channel_new(
      fl_engine_get_binary_messenger(fl_view_get_engine(view)),
      "com.wox.linux_window_manager", FL_METHOD_CODEC(codec));
  fl_method_channel_set_method_call_handler(channel, method_call_cb,
                                            g_object_ref(self), g_object_unref);

  // Store method channel reference for window events
  g_method_channel = channel;
  g_object_add_weak_pointer(G_OBJECT(channel), (gpointer *)&g_method_channel);

  // Add signal connection to implement rounded window
  g_signal_connect(window, "realize", G_CALLBACK(set_window_shape), NULL);
  g_signal_connect(window, "size-allocate", G_CALLBACK(on_size_allocate),
                   window);

  // Add signal connection for window focus-out event
  g_signal_connect(window, "focus-out-event", G_CALLBACK(on_window_focus_out),
                   self);

  gtk_widget_grab_focus(GTK_WIDGET(view));
}

// Implements GApplication::local_command_line.
static gboolean my_application_local_command_line(GApplication *application,
                                                  gchar ***arguments,
                                                  int *exit_status)
{
  MyApplication *self = MY_APPLICATION(application);
  // Strip out the first argument as it is the binary name.
  self->dart_entrypoint_arguments = g_strdupv(*arguments + 1);

  g_autoptr(GError) error = nullptr;
  if (!g_application_register(application, nullptr, &error))
  {
    g_warning("Failed to register: %s", error->message);
    *exit_status = 1;
    return TRUE;
  }

  g_application_activate(application);

  // hide at startup
  if (self->window != NULL)
  {
    gtk_widget_hide(GTK_WIDGET(self->window));
  }

  *exit_status = 0;

  return TRUE;
}

// Implements GApplication::startup.
static void my_application_startup(GApplication *application)
{
  G_APPLICATION_CLASS(my_application_parent_class)->startup(application);
}

// Implements GApplication::shutdown.
static void my_application_shutdown(GApplication *application)
{
  G_APPLICATION_CLASS(my_application_parent_class)->shutdown(application);
}

// Implements GObject::dispose.
static void my_application_dispose(GObject *object)
{
  MyApplication *self = MY_APPLICATION(object);
  g_clear_pointer(&self->dart_entrypoint_arguments, g_strfreev);

  // Clear method channel reference
  if (g_method_channel != nullptr)
  {
    g_object_remove_weak_pointer(G_OBJECT(g_method_channel),
                                 (gpointer *)&g_method_channel);
    g_method_channel = nullptr;
  }

  G_OBJECT_CLASS(my_application_parent_class)->dispose(object);
}

static void my_application_class_init(MyApplicationClass *klass)
{
  G_APPLICATION_CLASS(klass)->activate = my_application_activate;
  G_APPLICATION_CLASS(klass)->local_command_line =
      my_application_local_command_line;
  G_APPLICATION_CLASS(klass)->startup = my_application_startup;
  G_APPLICATION_CLASS(klass)->shutdown = my_application_shutdown;
  G_OBJECT_CLASS(klass)->dispose = my_application_dispose;
}

static void my_application_init(MyApplication *self)
{
  self->window = NULL;
  self->previous_active_window = 0;
  self->restore_previous_window_on_hide = FALSE;
}

MyApplication *my_application_new()
{
  g_set_prgname(APPLICATION_ID);
  return MY_APPLICATION(g_object_new(my_application_get_type(),
                                     "application-id", APPLICATION_ID, "flags",
                                     G_APPLICATION_NON_UNIQUE, nullptr));
}
