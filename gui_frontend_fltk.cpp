// gui_frontend_fltk.cpp - FLTK GUI frontend for Intuition Engine

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

(c) 2024 - 2025 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

#include <FL/Fl.H>
#include <FL/Fl_Window.H>
#include <FL/Fl_Button.H>
#include <FL/Fl_Text_Display.H>
#include <FL/Fl_Box.H>
#include <FL/Fl_File_Chooser.H>

static Fl_Window *window = nullptr;
static Fl_Window *about_window = nullptr;
static const char* selected_file = nullptr;
static bool should_execute = false;

void load_cb(Fl_Widget*, void*) {
    Fl_File_Chooser chooser(".", "Intuition Engine Executables (*.iex)", Fl_File_Chooser::SINGLE, "Load Program");
    chooser.show();
    while(chooser.shown()) { Fl::wait(); }
    if(chooser.value()) {
        selected_file = strdup(chooser.value());
        should_execute = true;
    }
}

void about_cb(Fl_Widget*, void*) {
    if (!about_window) {
        about_window = new Fl_Window(400, 200, "About Intuition Engine");
        Fl_Text_Display* text = new Fl_Text_Display(10, 10, 380, 150);
        Fl_Text_Buffer* buff = new Fl_Text_Buffer();
        text->buffer(buff);
        buff->text("Intuition Engine\n"
                   "(c) 2024 - 2025 Zayn Otley\n\n"
                   "https://github.com/intuitionamiga/IntuitionEngine\n\n"
                   "A modern 32-bit reimagining of the Commodore, Atari and Sinclair 8-bit home computers.");
        about_window->end();
    }
    about_window->show();
}

extern "C" {
    const char* get_selected_file() {
        return selected_file;
    }

    bool get_should_execute() {
        bool tmp = should_execute;
        should_execute = false;
        return tmp;
    }

    void create_window() {
        window = new Fl_Window(400, 100, "Intuition Engine - (c) 2024 - 2025 Zayn Otley");
        Fl_Button* load = new Fl_Button(10, 10, 70, 25, "Load");
        load->callback(load_cb);
        new Fl_Button(90, 10, 70, 25, "Reset");
        new Fl_Button(170, 10, 70, 25, "Debug");
        Fl_Button* about = new Fl_Button(250, 10, 70, 25, "About");
        about->callback(about_cb);
        window->end();
    }

    void show_window() {
        window->show();
        Fl::run();
    }
}