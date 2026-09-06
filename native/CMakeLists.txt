cmake_minimum_required(VERSION 3.22.1)
project(utsusemi)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# dobby: 关闭共享库，构建静态库供 zygisk so 链接
set(DOBBY_GENERATE_SHARED OFF CACHE BOOL "" FORCE)
add_subdirectory(dobby)

set(XDL_SOURCES
        zygisk/xdl/xdl.c
        zygisk/xdl/xdl_iterate.c
        zygisk/xdl/xdl_linker.c
        zygisk/xdl/xdl_lzma.c
        zygisk/xdl/xdl_util.c)

add_library(utsusemi_zygisk SHARED
        zygisk/main_zygisk.cpp
        zygisk/inject.cpp
        zygisk/config.cpp
        zygisk/child_gating.cpp
        zygisk/remapper.cpp
        ${XDL_SOURCES})

target_include_directories(utsusemi_zygisk PRIVATE
        zygisk
        zygisk/xdl/include
        zygisk/include)

# 与上游保持一致: 无异常/RTTI, 符号隐藏
target_compile_options(utsusemi_zygisk PRIVATE
        -fno-exceptions
        -fno-rtti
        -fvisibility=hidden
        -fvisibility-inlines-hidden)

target_link_libraries(utsusemi_zygisk dobby log)
