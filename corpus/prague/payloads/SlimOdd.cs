using CommonFeedModels.Enums;
using MessagePack;

namespace Common.IntegratedEventApi.Cache.Prague.Models;

[MessagePackObject]
public class SlimOdd
{
    public SlimOdd()
    {
        
    }

    public SlimOdd(string name,
        string? line,
        OddStatus status,
        OddStatus? statusManual,
        WinStatus winStatusId,
        WinStatus? winStatusIdManual,
        bool isDynamic,
        long timestamp,
        double oddsActual,
        double oddsFair,
        double? oddsManual,
        bool? isBoosted,
        bool? isBalancedLine,
        long? playerId)
    {
        Name = name;
        Line = line;
        Status = status;
        StatusManual = statusManual;
        WinStatusId = winStatusId;
        WinStatusIdManual = winStatusIdManual;
        IsDynamic = isDynamic;
        Timestamp = timestamp;
        OddsActual = oddsActual;
        OddsFair = oddsFair;
        OddsManual = oddsManual;
        IsBoosted = isBoosted;
        IsBalancedLine = isBalancedLine;
        PlayerId = playerId;
    }

    [Key(0)]
    public string Name { get; init; }

    [Key(3)]
    public bool IsDynamic { get; init; }

    [Key(4)]
    public long Timestamp { get; init; }

    [Key(6)]
    public string? Line { get; init; }

    [Key(8)]
    public double OddsActual { get; init; }

    [Key(9)]
    public double OddsFair { get; init; }

    [Key(10)]
    public double? OddsManual { get; init; }

    [Key(12)]
    public OddStatus Status { get; init; }

    [Key(13)]
    public OddStatus? StatusManual { get; init; }

    [Key(19)]
    public WinStatus WinStatusId { get; init; }

    [Key(20)]
    public WinStatus? WinStatusIdManual { get; init; }

    [Key(23)]
    public bool? IsBoosted { get; init; }

    [Key(26)]
    public bool? IsBalancedLine { get; init; }

    [Key(27)]
    public long? PlayerId { get; init; }
}
