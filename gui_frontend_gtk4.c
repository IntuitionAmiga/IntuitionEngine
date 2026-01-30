//go:build !headless

// gui_frontend_gtk4.c - GUI frontend for the Intuition Engine using GTK4

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

#include <gtk/gtk.h>
#include <string.h>
#include <gio/gio.h>

// Declare the Go functions we'll call
extern void do_reset(void);
extern const char* do_about(void);
extern void do_debug(void);

static GtkWidget *window = NULL;
static GtkApplication *app = NULL;
static const char* selected_file = NULL;
static int should_execute = 0;
static int start_minimized = 0;

static void file_chosen_cb(GObject *source_object, GAsyncResult *res, gpointer user_data) {
    GtkFileDialog *dialog = GTK_FILE_DIALOG(source_object);
    GError *error = NULL;
    GFile *file = gtk_file_dialog_open_finish(dialog, res, &error);

    if (file) {
        char *filename = g_file_get_path(file);
        if (selected_file != NULL) {
            free((void*)selected_file);
        }
        selected_file = strdup(filename);
        should_execute = 1;
        g_free(filename);
        g_object_unref(file);
    }
    g_object_unref(dialog);
}

static void load_cb(GtkWidget *widget, gpointer data) {
    GtkFileDialog *dialog = gtk_file_dialog_new();
    GtkFileFilter *filter = gtk_file_filter_new();
    gtk_file_filter_add_pattern(filter, "*.iex");
    gtk_file_filter_add_pattern(filter, "*.ie68");
    gtk_file_filter_add_pattern(filter, "*.ie65");
    gtk_file_filter_add_pattern(filter, "*.ie80");
    gtk_file_filter_add_pattern(filter, "*.ie86");
    gtk_file_filter_set_name(filter, "Intuition Engine Executables (*.iex, *.ie68, *.ie65, *.ie80, *.ie86)");

    GListStore *filters = g_list_store_new(GTK_TYPE_FILE_FILTER);
    g_list_store_append(filters, filter);
    gtk_file_dialog_set_filters(dialog, G_LIST_MODEL(filters));

    gtk_file_dialog_open(dialog, GTK_WINDOW(window), NULL, file_chosen_cb, NULL);
    g_object_unref(filters);
}

static void reset_cb(GtkWidget *widget, gpointer data) {
    do_reset();
}

static void debug_cb(GtkWidget *widget, gpointer data) {
    do_debug();
}

static void about_cb(GtkWidget *widget, gpointer data) {
    const char* title = "About";
    char* about_text = (char*)do_about();
    if (about_text != NULL) {
        GtkAlertDialog *dialog = gtk_alert_dialog_new(title);
        gtk_alert_dialog_set_detail(dialog, about_text);
        gtk_alert_dialog_show(dialog, GTK_WINDOW(window));
        // Free the memory allocated by Go's C.CString
        free(about_text);
        g_object_unref(dialog);
    }
}

const char* gtk_get_selected_file(void) {
    return selected_file;
}

int gtk_get_should_execute(void) {
    int tmp = should_execute;
    should_execute = 0;
    return tmp;
}

void gtk_set_start_minimized(int minimized) {
    start_minimized = minimized;
}

static void activate(GtkApplication *app, gpointer data) {
    window = gtk_application_window_new(app);
    gtk_window_set_title(GTK_WINDOW(window), "Intuition Engine");
    gtk_window_set_default_size(GTK_WINDOW(window), -1, -1);  // Shrink to fit content
    gtk_window_set_resizable(GTK_WINDOW(window), FALSE);       // Fixed size toolbar

    GtkWidget *box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 4);
    gtk_widget_set_margin_start(box, 6);
    gtk_widget_set_margin_end(box, 6);
    gtk_widget_set_margin_top(box, 6);
    gtk_widget_set_margin_bottom(box, 6);

    GtkWidget *load = gtk_button_new_with_label("Load");
    g_signal_connect(load, "clicked", G_CALLBACK(load_cb), NULL);

    GtkWidget *reset = gtk_button_new_with_label("Reset");
    g_signal_connect(reset, "clicked", G_CALLBACK(reset_cb), NULL);

    GtkWidget *debug = gtk_button_new_with_label("Debug");
    g_signal_connect(debug, "clicked", G_CALLBACK(debug_cb), NULL);

    GtkWidget *about = gtk_button_new_with_label("About");
    g_signal_connect(about, "clicked", G_CALLBACK(about_cb), NULL);

    gtk_box_append(GTK_BOX(box), load);
    gtk_box_append(GTK_BOX(box), reset);
    gtk_box_append(GTK_BOX(box), debug);
    gtk_box_append(GTK_BOX(box), about);

    gtk_window_set_child(GTK_WINDOW(window), box);
    gtk_window_present(GTK_WINDOW(window));

    // If running with a file argument, minimize the control window so it
    // doesn't obscure the display (Wayland doesn't allow window positioning)
    if (start_minimized) {
        gtk_window_minimize(GTK_WINDOW(window));
    }
}

void gtk_create_window(void) {
    app = gtk_application_new("org.intuition.engine", G_APPLICATION_DEFAULT_FLAGS);
    g_signal_connect(app, "activate", G_CALLBACK(activate), NULL);
}

void gtk_show_window(void) {
    g_application_run(G_APPLICATION(app), 0, NULL);
    g_object_unref(app);
}
