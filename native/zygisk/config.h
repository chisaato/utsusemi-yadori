#ifndef ZYGISKFRIDA_CONFIG_H
#define ZYGISKFRIDA_CONFIG_H

#include <memory>
#include <string>
#include <vector>
#include <optional>

// gadget 注入配置所在目录（staging 发布区，app 进程可读）
constexpr const char *kConfigDir = "/data/local/tmp/utsusemi";

struct child_gating_config {
    bool enabled;
    std::string mode;
    std::vector<std::string> injected_libraries;
};

struct target_config{
    bool enabled;
    std::string app_name;
    uint64_t start_up_delay_ms;
    std::vector<std::string> injected_libraries;
    child_gating_config child_gating;
};

std::optional<target_config> load_config(std::string const& module_dir, std::string const& app_name);

#endif  // ZYGISKFRIDA_CONFIG_H
