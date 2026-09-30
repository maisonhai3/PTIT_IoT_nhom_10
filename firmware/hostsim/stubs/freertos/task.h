#pragma once
#include "FreeRTOS.h"
struct TaskCtl;
typedef TaskCtl* TaskHandle_t;
typedef void (*TaskFunction_t)(void*);
BaseType_t xTaskCreatePinnedToCore(TaskFunction_t fn, const char* name, uint32_t stack, void* arg,
                                   UBaseType_t prio, TaskHandle_t* handle, BaseType_t core);
TickType_t xTaskGetTickCount();
void vTaskDelayUntil(TickType_t* prev, TickType_t increment);
uint32_t ulTaskNotifyTake(BaseType_t clearOnExit, TickType_t wait);
BaseType_t xTaskNotifyGive(TaskHandle_t handle);
