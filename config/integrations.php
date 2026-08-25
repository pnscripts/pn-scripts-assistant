<?php

return [
    /*
     * Smart-home platforms. Nothing is connected until you fill these in, and
     * the device tools report that plainly rather than failing obscurely.
     *
     * Home Assistant is the recommended route even for Tuya, Hue or Zigbee gear:
     * it already speaks all of them, so one integration here covers hardware
     * that would otherwise need an adapter each.
     *
     * The token is a long-lived access token from your Home Assistant profile
     * page. Treat it as a house key — it can unlock doors that HA controls.
     */
    'home_assistant' => [
        'url' => env('HOME_ASSISTANT_URL'),
        'token' => env('HOME_ASSISTANT_TOKEN'),
    ],
];
