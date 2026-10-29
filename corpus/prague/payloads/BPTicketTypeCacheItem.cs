using System.Text.Json.Serialization;
using Prague.Feed.Cache.Core;
using Common.EventApi.Models.Branded;
using CommonFeedModels.Enums;
using MessagePack;

namespace Common.EventApi.Cache.Prague.Models;

[MessagePackObject]
[DataCache(CacheClassName = nameof(Prague.Models.BPTicketTypeCache))]
[DataCacheTopic("Cache.[v:brandId].TicketType")]
public sealed partial class BPTicketTypeCacheItem
{
    [IgnoreMember]
    [JsonIgnore]
    public long TicketTypeId
    {
        get => Id;
        set => Id = value;
    }

    [Key(0)]
    [DataCacheKey]
    public (long EventId, long TicketTypeId) Key => (EventId, TicketTypeId);

    [Key(1)]
    [DataCacheIndex]
    public long EventId { get; set; }

    [Key(2)]
    public long Id { get; set; }

    [Key(3)]
    public string Name { get; set; } = string.Empty;

    [Key(4)]
    public string? ProviderSourceKey { get; set; }

    [Key(5)]
    public long ProviderKey { get; set; }

    [Key(6)]
    public TicketTypeStatus Status { get; set; }

    [Key(7)]
    public TicketTypeStatus? StatusManual { get; set; }

    [Key(8)]
    public TicketTypeStatus? StatusWatchDog { get; set; }

    [Key(9)]
    public bool IsLive { get; set; }

    [Key(10)]
    public bool IsListable { get; set; }

    [Key(11)]
    public bool IsResulted { get; set; }

    [Key(12)]
    public bool IsSettled { get; set; }

    [Key(13)]
    public bool IsManualChanged { get; set; }

    [Key(14)]
    public string AssignedSeatId { get; set; } = string.Empty;

    [Key(15)]
    public DateTime? ClosedTime { get; set; }

    [Key(16)]
    public DateTime? ResultedTime { get; set; }

    [Key(17)]
    public DateTime? SettledTime { get; set; }

    [Key(18)]
    public double FeedMargin { get; set; }

    [Key(19)]
    public double ActualMargin { get; set; }

    [Key(20)]
    public double? ActualMarginManual { get; set; }

    [Key(21)]
    public decimal? LimitManualSingle { get; set; }

    [Key(22)]
    public decimal? LimitManualBundle { get; set; }

    [Key(23)]
    public decimal? LimitManualPurchaseBuilder { get; set; }

    [Key(24)]
    public long Timestamp { get; set; }

    [Key(25)]
    public List<Quote> Quotes { get; set; } = [];

    [Key(26)]
    public List<string> Groups { get; set; } = [];

    [Key(27)]
    public Quote? T { get; set; }
}