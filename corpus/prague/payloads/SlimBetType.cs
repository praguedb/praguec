using Prague.Feed.Cache.Core;
using CommonFeedModels.Enums;
using MessagePack;

namespace Common.EventApi.Cache.Prague.Models;

[MessagePackObject]
[DataCache(CacheClassName = nameof(SlimBetTypeCache))]
[DataCacheTopic("Cache.[v:brandId].BetType")]
public sealed partial class SlimBetType
{
    public SlimBetType()
    {
        
    }

    public SlimBetType(long betTypeId,
        long eventId,
        string name,
        long timestamp,
        bool isLive,
        bool isLineable,
        BetTypeStatus status,
        BetTypeStatus? statusManual,
        decimal? limitManualSingle,
        decimal? limitManualCombo,
        decimal? limitManualBetBuilder)
    {
        BetTypeId = betTypeId;
        EventId = eventId;
        Name = name;
        Timestamp = timestamp;
        IsLive = isLive;
        IsLineable = isLineable;
        Status = status;
        StatusManual = statusManual;
        LimitManualSingle = limitManualSingle;
        LimitManualCombo = limitManualCombo;
        LimitManualBetBuilder = limitManualBetBuilder;
    }

    [Key(0)]
    [DataCacheKey]
    public (long EventId, long BetTypeId) Id => (EventId, BetTypeId);

    [Key(1)]
    [DataCacheIndex]
    public long EventId { get; set; }

    [Key(2)]
    [DataCacheIndex]
    public long BetTypeId { get; set; }

    [Key(3)]
    public string Name { get; set; } = string.Empty;

    [Key(6)]
    [DataCacheIndex]
    public BetTypeStatus Status { get; set; }

    [Key(7)]
    public BetTypeStatus? StatusManual { get; set; }

    [Key(8)]
    public BetTypeStatus? StatusWatchDog { get; set; }

    [Key(9)]
    [DataCacheIndex]
    public bool IsLive { get; set; }

    [Key(10)]
    public bool IsLineable { get; set; }

    [Key(21)]
    public decimal? LimitManualSingle { get; set; }

    [Key(22)]
    public decimal? LimitManualCombo { get; set; }

    [Key(23)]
    public decimal? LimitManualBetBuilder { get; set; }

    [Key(24)]
    [DataCacheIndex(DataCacheIndexType.Range)]
    [DataCacheFromTimestamp]
    public long Timestamp { get; set; }

    [Key(25)]
    public SlimOdd[] Odds { get; set; } = [];
}
