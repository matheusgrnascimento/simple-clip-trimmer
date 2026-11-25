//! What this setup knows about the app it installs.

use windows::core::{w, PCWSTR};

pub const APP_NAME: &str = "Simple Clip Trimmer";
/// The app's executable inside the payload, and in the install folder.
pub const EXE: &str = "Simple Clip Trimmer.exe";
pub const PUBLISHER: &str = "Matheus Nascimento";
pub const URL: &str = "https://github.com/matheusgrnascimento/simple-clip-trimmer";
/// The Start menu shortcut's tooltip.
pub const DESCRIPTION: &str = "Trim, resize and compress videos";
/// The name of the key in `Uninstall` (Settings > Apps).
pub const UNINSTALL_KEY_NAME: &str = "SimpleClipTrimmer";
/// The upgrade code of the MSI, to remove a copy installed by it.
pub const MSI_UPGRADE_CODE: PCWSTR = w!("{6B1F3C52-8D0E-4A57-9C3E-2F7A41D5B9E8}");
/// The folder under `%APPDATA%` with what the user created, if the app has one.
pub const SETTINGS_FOLDER: Option<&str> = None;
pub const KEEP_SETTINGS: &str = "Keep my settings";
/// Window class of the setup, and the variable that forces `light` or `dark`.
pub const WINDOW_CLASS: PCWSTR = w!("SimpleClipTrimmer.Setup");
pub const THEME_VARIABLE: &str = "SIMPLE_CLIP_TRIMMER_THEME";

pub const SETUP_TITLE: PCWSTR = w!("Simple Clip Trimmer Setup");
pub const UNINSTALL_TITLE: PCWSTR = w!("Uninstall Simple Clip Trimmer");

pub const INSTALL_INTRO: &str = "Trim, resize and compress videos. Installs for your account only, with no administrator rights. FFmpeg must be in PATH.";
pub const UPDATE_INTRO: &str = "Updates the app in place.";
pub const UNINSTALL_INTRO: &str = "Removes the app and its Start menu shortcut. Your videos stay.";
pub const INSTALLED: &str = "Installed. Find Simple Clip Trimmer in the Start menu.";
pub const REMOVED: &str = "Simple Clip Trimmer was removed.";
