#pragma once
#include "FreeRTOS.h"
struct MutexCtl;
typedef MutexCtl* SemaphoreHandle_t;
SemaphoreHandle_t xSemaphoreCreateMutex();
BaseType_t xSemaphoreTake(SemaphoreHandle_t m, TickType_t wait);
BaseType_t xSemaphoreGive(SemaphoreHandle_t m);
