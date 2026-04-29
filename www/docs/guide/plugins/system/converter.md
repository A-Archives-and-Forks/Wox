# Converter Plugin

Converter handles units, currencies, crypto prices, number bases, dates, time zones, and simple math with typed values.

## Quick Start

```text
1km to m
100lb to kg
255 to hex
1 btc to usd
100 usd + 50 usd
```

Converter listens globally. Use `calculator` as an explicit keyword if another global result is taking priority.

![Converter plugin result list](/images/system-plugin-converter.png)

## Supported Work

| Type | Examples |
| --- | --- |
| Units | length, weight, temperature, time |
| Number base | binary, octal, decimal, hexadecimal |
| Currency | common fiat currencies |
| Crypto | common crypto symbols such as BTC, ETH, USDT, and BNB |
| Time | timestamps, dates, durations, and time zones |
| Math | `+`, `-`, `*`, `/` with compatible values |

## Tips

- Use `to` or `in` to make the target unit explicit.
- Base conversion expects an integer and a target base.
- Currency and crypto rates refresh in the background and may use cached values while offline.
- Set your default currency in plugin settings if fallback conversions are not what you expect.
