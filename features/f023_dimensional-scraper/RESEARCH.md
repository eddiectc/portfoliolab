# Research: Dimensional Fund Data Extraction

This document outlines the discovery and extraction strategy for fund data from the Dimensional website (`dimensional.com`).

## Extraction Strategy: API-First Approach

Dimensional provides a set of internal JSON APIs that can be accessed without a headless browser, provided the correct headers are sent. This approach is significantly more robust and faster than DOM scraping.

### 1. Fund Discovery & Mapping
To start, we need to map a fund's **ISIN** to its internal **`portfolioNumber`**.

- **Endpoint:** `GET https://etf.dimensional.com/public/v2/fundcenter?allowMorningstarFixedIncome=true`
- **Required Header:** `x-selected-country: GB` (or other ISO country code)
- **Fingerprinting:** Always use a realistic browser user-agent and TLS fingerprint to avoid being blocked.
- **Workflow:**
  1. Fetch the fund center registry.

  2. Iterate through the `portfolios` array.
  3. Find the portfolio where `meta.identifiers` contains an entry with `slug: "isin"` and the target ISIN value.
  4. Extract the corresponding `portfolioNumber` (e.g., `IE000EGGFVG6` $\rightarrow$ `1600`).

### 2. Detailed Fund Data Extraction
Once the `portfolioNumber` is known, almost all fund-specific data can be retrieved via a single POST request.

- **Endpoint:** `POST https://etf.dimensional.com/public/v2/fundcenter/funddetail`
- **Required Header:** `x-selected-country: GB`
- **Request Body:** `{"portfolioNumber": <PORTFOLIO_NUMBER>}`
- **Data Mapping (within `lensGroups`):**

| Target Data | API Slug (`lensGroups[].data.slug`) | Path / Field |
| :--- | :--- | :--- |
| **Fund Facts** | `fundFacts` | `blends[0].data.fundFacts` $\rightarrow$ `marketingName`, `benchmarks`, `fundAum`, `inceptionDate` |
| **NAV / Prices** | `fundPrices` | `blends[0].data.fundPrices.prices[0]` $\rightarrow$ `nav` |
| **Fees** | `fees` | `blends[0].data.fees.fees` $\rightarrow$ find `slug: "net-exp-ratio"` |
| **Sector Allocation** | `charsEquityAllocationByGics` | `blends[0].data.allocations` $\rightarrow$ list of `{name, weight}` |
| **Country Allocation** | `charsEquityAllocationByCountryByRegionDevelopedEmerging` | `blends[0].data.allocations` $\rightarrow$ `subCategories` for "Developed" and "Emerging" |
| **Holdings Metadata**| `charsEtfTopHoldingsDaily` | `blends[0].data` $\rightarrow$ `asOfDate`, `fullHoldingsCsvUrl` |

### 3. Holdings Extraction (CSV)
Holdings are provided as static CSV files.

- **Workflow:**
  1. Retrieve the `fullHoldingsCsvUrl` from the `funddetail` API.
  2. Fetch the CSV file directly.
  3. Parse the CSV for holdings data.

### 4. Performance Data
Performance returns are available in the **Fund Center API** (`fundcenter`).

- **Path:** `portfolios[].returnsMonthly` and `portfolios[].returnsDaily`
- **Fields:** `annualizedReturn1Year`, `annualizedReturn5Year`, `annualizedReturnYtd`, etc.

---

## Summary of API Workflow

1. **Step 1 (Registry):** `GET /public/v2/fundcenter` $\xrightarrow{ISIN}$ `portfolioNumber`.
2. **Step 2 (Details):** `POST /public/v2/fundcenter/funddetail` (`portfolioNumber`) $\rightarrow$ Metadata, Fees, Allocations, NAV, and CSV URL.
3. **Step 3 (Holdings):** `GET <fullHoldingsCsvUrl>` $\rightarrow$ Full holdings list.
