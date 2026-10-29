using Prague.Feed.Cache.Core;
using Common.EventApi.Models.Enums;
using CommonFeedModels.Enums;
using MessagePack;

namespace BO.Services.Prague.Models;

/// <summary>
/// TBD: Handling of changes from BO UI
/// </summary>
[MessagePackObject]
[DataCache(CacheClassName = nameof(BOSearchCache))]
[DataCacheTopic("Cache.[v:brandId].Search")]
public sealed partial class BOSearchCacheItem
{
    [Key(0)]
    [DataCacheKey]
    public (long EventId, Channel ChannelId) Key => (EventId, ChannelId);

    [Key(1)]
    [DataCacheIndex]
    public long EventId { get; set; }

    [Key(2)]
    [DataCacheIndex]
    public Channel ChannelId { get; set; }

    [Key(3)]
    public string? HeadlinerDisplayName { get; set; }

    [Key(4)]
    public long? OpenerId { get; set; }

    [Key(5)]
    public string? OpenerName { get; set; }

    [Key(6)]
    public string? OpenerDisplayName { get; set; }

    [Key(7)]
    public EventStatus EventStatus { get; set; }

    [Key(8)]
    public EventStatus? EventStatusManual { get; set; }

    [Key(9)]
    public EventStatus? EventStatusWatchDog { get; set; }

    [Key(10)]
    public WatchDogAction WatchDogAction { get; set; }

    [Key(11)]
    public BookingStatus BookingStatus { get; set; }

    [Key(12)]
    public bool IsTopEvent { get; set; }

    [Key(13)]
    public int TopEventOrder { get; set; }

    [Key(14)]
    public int ActiveTicketTypesCount { get; set; }

    [Key(15)]
    public int TotalTicketTypesCount { get; set; }

    [Key(17)]
    public int EventTypeId { get; set; }

    [Key(18)]
    public int? ListingId { get; set; }

    [Key(19)]
    public bool IsLive { get; set; }

    [Key(20)]
    public string? LiveObjectMinute { get; set; }

    [Key(21)]
    public bool? IsPinned { get; set; }

    [Key(22)]
    public bool IsContainsPendingQuotesWithPurchases { get; set; }

    [Key(23)]
    public bool IsManualChanged { get; set; }

    [Key(24)]
    public string? HeadlinerName { get; set; }

    [Key(25)]
    public bool HasIsFeaturedQuotes { get; set; }

    [Key(26)]
    public long? HeadlinerId { get; set; }

    [Key(27)]
    public int? TourDisplayOrder { get; set; }

    [Key(30)]
    public string EventName { get; set; }

    [Key(31)]
    public DateTime EventDate { get; set; }

    [Key(32)]
    public long? Timestamp { get; set; }

    [Key(33)]
    public long GenreId { get; set; }

    [Key(34)]
    public string GenreName { get; set; }

    [Key(35)]
    public string? GenreDisplayName { get; set; }

    [Key(36)]
    public int? GenreDisplayOrder { get; set; }

    [Key(37)]
    public bool GenreDisabled { get; set; }

    [Key(38)]
    public long CountryId { get; set; }

    [Key(39)]
    public string CountryName { get; set; }

    [Key(40)]
    public string? CountryDisplayName { get; set; }

    [Key(41)]
    public int? CountryDisplayOrder { get; set; }

    [Key(42)]
    public bool CountryDisabled { get; set; }

    [Key(43)]
    public long TourId { get; set; }

    [Key(44)]
    public string TourName { get; set; }

    [Key(45)]
    public string? TourDisplayName { get; set; }

    [Key(46)]
    public bool TourDisabled { get; set; }

    [Key(47)]
    public bool HasTicketTypes { get; set; }
}