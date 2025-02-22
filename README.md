# EverythingX

## Overview
EverythingX is a _blazing_ fast file name search tool for MacOS and Linux. 


## How is EverythingX different from other search engines such as Spotlight on MacOS and find on Linux
- Minimal resource usage
- Quick file indexing
- Real-time updating
- Clean and simple user interface
- Quick searching
- Quick startup
- Open source

A background service maintains real-time updates as files and directories change.  An app and a command-line tool do fast searches as you type.

EverythingX attempts to replicate the very excellent Windows utility called [Everything by Voidtools](https://www.voidtools.com/support/everything/).  

## Command Line Interface (CLI)
The EverythingX CLI, called ```ev```, allows you to search the database for files and directories from the command-line. It is far faster than using ```find``` but has fewer options.  Pipe the output to grep or other to filter results.

### Usage
```
ev -n search_term [-b]

-b bold search term in the result. This option helps readability of the output but interferes with piping results to another command.
```

```ev -b -n bashrc                  
/System/Library/Templates/Data/private/etc/bashrc
/System/Library/Templates/Data/private/etc/bashrc_Apple_Terminal
/System/Volumes/Update/mnt1/System/Library/Templates/Data/private/etc/bashrc
/System/Volumes/Update/mnt1/System/Library/Templates/Data/private/etc/bashrc_Apple_Terminal
/private/etc/bashrc
/private/etc/bashrc_Apple_Terminal
```

## EverythingX App
```everythingx``` is a GUI application that provides an intuitive way to search and manage files on your system.

Instant search results as you type to find full file paths and details.

### Installation

## Background Service
The EverythingX background service continuously indexes your files to ensure fast and accurate search results.

### Features
- **Automatic indexing**: Keeps your file index up-to-date.
- **Low resource usage**: Optimized to run efficiently in the background.
- **Configurable**: Customize indexing settings to match your needs.

## License
EverythingX is licensed under the MIT License. See the [LICENSE](LICENSE) file for more information.

## Contact, feature requests, and bug reports
Create an issue on the [Github Page](https://github.com/AlanKK/everythingx/issues)
