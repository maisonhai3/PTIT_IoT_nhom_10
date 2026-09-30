#pragma once
#include <stdbool.h>
#include <stdint.h>
#include "freertos/task.h"
typedef int esp_err_t;
esp_err_t esp_task_wdt_init(uint32_t timeout_s, bool panic);
esp_err_t esp_task_wdt_add(TaskHandle_t handle);
esp_err_t esp_task_wdt_reset(void);
