using Prague.Feed.Cache.Core;
using Common.EventApi.Models.Branded;
using Common.EventApi.Models.Enums;
using CommonFeedModels.Enums;
using MessagePack;

namespace Common.EventApi.Cache.Prague.Models;

[MessagePackObject]
[DataCache(CacheClassName = nameof(SlimShowCache))]
[DataCacheTopic("Cache.[v:brandId].Show")]
public sealed partial class SlimShow
{
    [Key(0)]
    [DataCacheKey]
    public long Id { get; init; }

    // Identifiers
    [Key(1)]
    [DataCacheIndex]
    public long GenreId { get; set; }

    [Key(2)]
    [DataCacheIndex]
    public long CountryId { get; set; }

    [Key(3)]
    [DataCacheIndex]
    public long TourId { get; set; }

    [Key(4)]
    public long? HeadlinerId { get; set; }

    [Key(5)]
    public long? OpenerId { get; set; }

    [Key(7)]
    public long? StatisticsCenterId { get; set; }

    // Provider/meta
    [Key(8)]
    public string Name { get; set; } = string.Empty;

    [Key(9)]
    public string? ProviderSourceKey { get; set; }

    [Key(10)]
    public long ProviderKey { get; set; }

    [Key(11)]
    public byte ProviderId { get; set; }

    [Key(12)]
    [DataCacheIndex]
    public int EventTypeId { get; set; }

    [Key(14)]
    public int? ListingId { get; set; }

    [Key(15)]
    [DataCacheIndex]
    public bool IsLive { get; set; }

    [Key(18)]
    public bool IsManualChanged { get; set; }

    [Key(19)]
    public bool? IsPinned { get; set; }

    [Key(20)]
    public bool IsContainsPendingQuotesWithPurchases { get; set; }

    [Key(21)]
    [DataCacheIndex]
    public WatchDogAction WatchDogAction { get; set; }

    [Key(23)]
    [DataCacheIndex]
    public EventStatus EventStatus { get; set; }

    [Key(24)]
    public EventStatus? EventStatusManual { get; set; }

    [Key(25)]
    public EventStatus? EventStatusWatchDog { get; set; }
    
    [Key(26)]
    [DataCacheIndex]
    public BookingStatus BookingStatus { get; set; }

    [Key(27)]
    [DataCacheIndex(DataCacheIndexType.Range)]
    public DateTime EventDate { get; set; }

    [Key(29)]
    public DateTime? EventDateManual { get; set; }

    [Key(32)]
    public int Version { get; set; }

    [Key(33)]
    [DataCacheIndex(DataCacheIndexType.Range)]
    public long InternalTimestamp { get; set; }

    [Key(34)]
    public long ProviderTimestamp { get; set; }

    // Limits
    [Key(35)]
    public decimal? LiveLimitManualSingle { get; set; }

    [Key(36)]
    public decimal? PresaleLimitManualSingle { get; set; }

    [Key(37)]
    public decimal? LiveLimitManualBundle { get; set; }

    [Key(38)]
    public decimal? PresaleLimitManualBundle { get; set; }

    [Key(39)]
    public decimal? LivePurchaseBuilderManualLimit { get; set; }

    [Key(40)]
    public decimal? PresalePurchaseBuilderManualLimit { get; set; }

    [Key(44)]
    public bool HasTicketTypes { get; set; }

    // Coverage
    [Key(47)]
    public string? CoverageSource { get; set; }

    [Key(53)]
    public EventSource? EventSource { get; set; }

    [Key(54)]
    public StaticDataRef Genre { get; set; } = null!;

    [Key(55)]
    public StaticDataRef Country { get; set; } = null!;

    [Key(56)]
    public StaticDataRef Tour { get; set; } = null!;

    [Key(57)]
    public StaticDataRef Headliner { get; set; } = null!;

    [Key(58)]
    public StaticDataRef Opener { get; set; } = null!;
}
