package com.blindmap

import kotlin.math.min
import kotlin.math.pow
import kotlin.test.Test
import kotlin.test.assertEquals

class ReconnectManagerTest {
    @Test
    fun backoffDelays() {
        val delays = (0 until 5).map { attempt ->
            min(2.0.pow(attempt).toLong() * 1000L, 30_000L)
        }
        assertEquals(listOf(1000L, 2000L, 4000L, 8000L, 16000L), delays)
    }
}
